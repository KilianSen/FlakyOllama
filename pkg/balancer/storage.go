package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// PerformanceMetric stores historical data for a node and model.
type PerformanceMetric struct {
	NodeID      string
	ModelName   string
	AvgLatency  float64
	SuccessRate float64
	LastUpdated time.Time
}

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	// Enable WAL mode and busy timeout for better concurrency
	dsn := path
	if path != ":memory:" {
		dsn += "?_journal=WAL&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Create tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS metrics (
			node_id TEXT,
			model_name TEXT,
			latency REAL,
			success INTEGER,
			timestamp DATETIME,
			PRIMARY KEY (node_id, model_name, timestamp)
		);`,
		`CREATE TABLE IF NOT EXISTS model_metadata (
			model_name TEXT PRIMARY KEY,
			min_vram uint64,
			updated_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics (timestamp);`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME,
			node_id TEXT,
			level TEXT,
			component TEXT,
			message TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs (timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_node ON logs (node_id);`,
		`CREATE TABLE IF NOT EXISTS model_requests (
			id TEXT PRIMARY KEY,
			type TEXT,
			model TEXT,
			node_id TEXT,
			status TEXT,
			agent_task_id TEXT,
			requested_at DATETIME,
			approved_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS model_policies (
			model TEXT,
			node_id TEXT,
			is_banned BOOLEAN DEFAULT 0,
			is_pinned BOOLEAN DEFAULT 0,
			is_persistent BOOLEAN DEFAULT 0,
			PRIMARY KEY (model, node_id)
		);`,
		`CREATE TABLE IF NOT EXISTS token_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME,
			node_id TEXT,
			model TEXT,
			input_tokens INTEGER,
			output_tokens INTEGER,
			reward REAL DEFAULT 0,
			cost REAL DEFAULT 0,
			ttft_ms INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			client_key TEXT,
			user_id TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_timestamp ON token_usage (timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_user ON token_usage (user_id);`,
		`CREATE TABLE IF NOT EXISTS client_keys (
			key TEXT PRIMARY KEY,
			label TEXT,
			quota_limit INTEGER DEFAULT -1,
			quota_used INTEGER DEFAULT 0,
			credits REAL DEFAULT 0,
			active BOOLEAN DEFAULT 1,
			user_id TEXT,
			status TEXT DEFAULT 'active'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_client_keys_user ON client_keys (user_id);`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			sub TEXT UNIQUE,
			email TEXT,
			name TEXT,
			is_admin BOOLEAN DEFAULT 0,
			quota_limit INTEGER DEFAULT -1,
			quota_used INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS agent_keys (
			key TEXT PRIMARY KEY,
			label TEXT,
			node_id TEXT,
			balancer_token TEXT DEFAULT '',
			credits_earned REAL DEFAULT 0,
			reputation REAL DEFAULT 1.0,
			active BOOLEAN DEFAULT 1,
			user_id TEXT,
			status TEXT DEFAULT 'active'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_keys_user ON agent_keys (user_id);`,
		`CREATE TABLE IF NOT EXISTS user_model_policies (
			user_id TEXT,
			model TEXT,
			reward_factor REAL DEFAULT 1.0,
			cost_factor REAL DEFAULT 1.0,
			is_disabled BOOLEAN DEFAULT 0,
			PRIMARY KEY (user_id, model)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_token_usage_node ON token_usage (node_id);`,
	}
	for _, q := range queries {
		if _, err = db.Exec(q); err != nil {
			return nil, err
		}
	}

	// Schema migrations — attempt additive changes and ignore "duplicate column" errors
	// so existing databases are upgraded without data loss.
	migrations := []string{
		`ALTER TABLE agent_keys ADD COLUMN balancer_token TEXT DEFAULT ''`,
	}
	for _, m := range migrations {
		if _, err = db.Exec(m); err != nil {
			// SQLite returns "duplicate column name" if the column already exists; that's fine.
			if !strings.Contains(err.Error(), "duplicate column name") {
				return nil, fmt.Errorf("migration failed (%s): %w", m, err)
			}
		}
	}

	return &SQLiteStorage{db: db}, nil
}

func (s *SQLiteStorage) CreateModelRequest(req models.ModelRequest) error {
	_, err := s.db.Exec(`
		INSERT INTO model_requests (id, type, model, node_id, status, agent_task_id, requested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Type, req.Model, req.NodeID, req.Status, req.AgentTaskID, req.RequestedAt)
	return err
}

func (s *SQLiteStorage) GetModelRequest(id string) (models.ModelRequest, error) {
	var req models.ModelRequest
	var approvedAt sql.NullTime
	var taskID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, type, model, node_id, status, agent_task_id, requested_at, approved_at 
		FROM model_requests WHERE id = ?`, id).
		Scan(&req.ID, &req.Type, &req.Model, &req.NodeID, &req.Status, &taskID, &req.RequestedAt, &approvedAt)
	if err != nil {
		return req, err
	}
	if approvedAt.Valid {
		req.ApprovedAt = &approvedAt.Time
	}
	if taskID.Valid {
		req.AgentTaskID = taskID.String
	}
	return req, nil
}

func (s *SQLiteStorage) ListModelRequests(status models.ModelRequestStatus) ([]models.ModelRequest, error) {
	query := `SELECT id, type, model, node_id, status, agent_task_id, requested_at, approved_at FROM model_requests`
	var rows *sql.Rows
	var err error
	if status != "" {
		query += ` WHERE status = ?`
		rows, err = s.db.Query(query, status)
	} else {
		rows, err = s.db.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reqs := make([]models.ModelRequest, 0)
	for rows.Next() {
		var req models.ModelRequest
		var approvedAt sql.NullTime
		var taskID sql.NullString
		if err := rows.Scan(&req.ID, &req.Type, &req.Model, &req.NodeID, &req.Status, &taskID, &req.RequestedAt, &approvedAt); err != nil {
			return nil, err
		}
		if approvedAt.Valid {
			req.ApprovedAt = &approvedAt.Time
		}
		if taskID.Valid {
			req.AgentTaskID = taskID.String
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

func (s *SQLiteStorage) UpdateModelRequestStatus(id string, status models.ModelRequestStatus) error {
	var approvedAt interface{}
	if status == models.StatusApproved {
		approvedAt = time.Now()
	} else {
		approvedAt = nil
	}

	if status == models.StatusApproved {
		_, err := s.db.Exec("UPDATE model_requests SET status = ?, approved_at = ? WHERE id = ?", status, approvedAt, id)
		return err
	}

	_, err := s.db.Exec("UPDATE model_requests SET status = ? WHERE id = ?", status, id)
	return err
}

func (s *SQLiteStorage) UpdateModelRequestTaskID(id, taskID string) error {
	_, err := s.db.Exec("UPDATE model_requests SET agent_task_id = ? WHERE id = ?", taskID, id)
	return err
}

func (s *SQLiteStorage) SetModelPolicy(model, nodeID string, banned, pinned, persistent bool) error {
	_, err := s.db.Exec(`
		INSERT INTO model_policies (model, node_id, is_banned, is_pinned, is_persistent) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(model, node_id) DO UPDATE SET is_banned = excluded.is_banned, is_pinned = excluded.is_pinned, is_persistent = excluded.is_persistent`,
		model, nodeID, banned, pinned, persistent)
	return err
}

func (s *SQLiteStorage) RecordTokenUsage(nodeID, model string, input, output int, reward, cost float64, ttft, duration int64, clientKey string) error {
	return s.RecordTokenUsageBatch([]struct {
		NodeID, Model, ClientKey, UserID string
		Input, Output                    int
		Reward, Cost                     float64
		TTFT, Duration                   int64
	}{{nodeID, model, clientKey, "", input, output, reward, cost, ttft, duration}})
}

func (s *SQLiteStorage) RecordTokenUsageBatch(entries []struct {
	NodeID, Model, ClientKey, UserID string
	Input, Output                    int
	Reward, Cost                     float64
	TTFT, Duration                   int64
}) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, e := range entries {
		targetUserID := e.UserID

		// 1. Update Quotas and get UserID if not provided
		if e.ClientKey != "" {
			var dbUserID sql.NullString
			err = tx.QueryRow(`UPDATE client_keys SET quota_used = quota_used + ?, credits = credits - ? WHERE key = ? RETURNING user_id`,
				int64(e.Input+e.Output), e.Cost, e.ClientKey).Scan(&dbUserID)
			if err != nil {
				return err
			}
			if targetUserID == "" && dbUserID.Valid && dbUserID.String != "" {
				targetUserID = dbUserID.String
			}
		}

		if targetUserID != "" {
			_, err = tx.Exec(`UPDATE users SET quota_used = quota_used + ? WHERE id = ?`, int64(e.Input+e.Output), targetUserID)
			if err != nil {
				return err
			}
		}

		// 2. Record the usage
		_, err = tx.Exec(`
			INSERT INTO token_usage (timestamp, node_id, model, input_tokens, output_tokens, reward, cost, ttft_ms, duration_ms, client_key, user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			time.Now(), e.NodeID, e.Model, e.Input, e.Output, e.Reward, e.Cost, e.TTFT, e.Duration, e.ClientKey, targetUserID)
		if err != nil {
			return err
		}

		// 3. Update Agent Key Reward
		_, err = tx.Exec(`UPDATE agent_keys SET credits_earned = credits_earned + ? WHERE key = ?`,
			e.Reward, e.NodeID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStorage) GetTotalTokenStats() (map[string]struct {
	Input, Output int64
	Reward        float64
	Cost          float64
}, error) {
	rows, err := s.db.Query("SELECT node_id, SUM(input_tokens), SUM(output_tokens), SUM(reward), SUM(cost) FROM token_usage GROUP BY node_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]struct {
		Input, Output int64
		Reward        float64
		Cost          float64
	})
	for rows.Next() {
		var nodeID string
		var input, output int64
		var reward, cost float64
		if err := rows.Scan(&nodeID, &input, &output, &reward, &cost); err != nil {
			return nil, err
		}
		stats[nodeID] = struct {
			Input, Output int64
			Reward        float64
			Cost          float64
		}{Input: input, Output: output, Reward: reward, Cost: cost}
	}
	return stats, nil
}

// GetRecentThroughput returns tokens/sec per node over the last windowSeconds seconds.
func (s *SQLiteStorage) GetRecentThroughput(windowSeconds int64) (map[string]float64, error) {
	rows, err := s.db.Query(`
		SELECT node_id, CAST(SUM(input_tokens + output_tokens) AS REAL) / ?
		FROM token_usage
		WHERE timestamp > datetime('now', '-' || ? || ' seconds')
		GROUP BY node_id`,
		windowSeconds, windowSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]float64)
	for rows.Next() {
		var nodeID string
		var tps float64
		if err := rows.Scan(&nodeID, &tps); err != nil {
			return nil, err
		}
		result[nodeID] = tps
	}
	return result, nil
}

func (s *SQLiteStorage) GetPerformanceAnalytics() (map[string]struct {
	AvgTTFT     float64
	AvgDuration float64
	Requests    int
}, error) {
	rows, err := s.db.Query(`
		SELECT model, AVG(ttft_ms), AVG(duration_ms), COUNT(*) 
		FROM token_usage 
		WHERE timestamp >= datetime('now', '-24 hours')
		GROUP BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	analytics := make(map[string]struct {
		AvgTTFT     float64
		AvgDuration float64
		Requests    int
	})
	for rows.Next() {
		var model string
		var ttft, duration float64
		var count int
		if err := rows.Scan(&model, &ttft, &duration, &count); err != nil {
			return nil, err
		}
		analytics[model] = struct {
			AvgTTFT     float64
			AvgDuration float64
			Requests    int
		}{AvgTTFT: ttft, AvgDuration: duration, Requests: count}
	}
	return analytics, nil
}

// Client Key Management
func (s *SQLiteStorage) CreateClientKey(k models.ClientKey) error {
	_, err := s.db.Exec(`INSERT INTO client_keys (key, label, quota_limit, credits, user_id, status) VALUES (?, ?, ?, ?, ?, ?)`,
		k.Key, k.Label, k.QuotaLimit, k.Credits, k.UserID, k.Status)
	return err
}

func (s *SQLiteStorage) GetClientKey(key string) (models.ClientKey, error) {
	var k models.ClientKey
	var userID sql.NullString
	err := s.db.QueryRow(`SELECT key, label, quota_limit, quota_used, credits, active, user_id, status FROM client_keys WHERE key = ?`, key).
		Scan(&k.Key, &k.Label, &k.QuotaLimit, &k.QuotaUsed, &k.Credits, &k.Active, &userID, &k.Status)
	if err != nil {
		return k, err
	}
	if userID.Valid {
		k.UserID = userID.String
	}
	return k, nil
}

func (s *SQLiteStorage) ListClientKeys() ([]models.ClientKey, error) {
	rows, err := s.db.Query(`SELECT key, label, quota_limit, quota_used, credits, active, user_id, status FROM client_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]models.ClientKey, 0)
	for rows.Next() {
		var k models.ClientKey
		var userID sql.NullString
		if err := rows.Scan(&k.Key, &k.Label, &k.QuotaLimit, &k.QuotaUsed, &k.Credits, &k.Active, &userID, &k.Status); err != nil {
			return nil, err
		}
		if userID.Valid {
			k.UserID = userID.String
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// User Management
func (s *SQLiteStorage) CreateUser(u models.User) error {
	_, err := s.db.Exec(`INSERT INTO users (id, sub, email, name, is_admin, quota_limit, quota_used) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Sub, u.Email, u.Name, u.IsAdmin, u.QuotaLimit, u.QuotaUsed)
	return err
}

func (s *SQLiteStorage) GetUserBySub(sub string) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(`SELECT id, sub, email, name, is_admin, quota_limit, quota_used FROM users WHERE sub = ?`, sub).
		Scan(&u.ID, &u.Sub, &u.Email, &u.Name, &u.IsAdmin, &u.QuotaLimit, &u.QuotaUsed)
	return u, err
}

func (s *SQLiteStorage) GetUserByID(id string) (models.User, error) {
	var u models.User
	err := s.db.QueryRow(`SELECT id, sub, email, name, is_admin, quota_limit, quota_used FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Sub, &u.Email, &u.Name, &u.IsAdmin, &u.QuotaLimit, &u.QuotaUsed)
	return u, err
}

func (s *SQLiteStorage) UpdateUser(u models.User) error {
	_, err := s.db.Exec(`UPDATE users SET email = ?, name = ?, is_admin = ?, quota_limit = ?, quota_used = ? WHERE id = ?`,
		u.Email, u.Name, u.IsAdmin, u.QuotaLimit, u.QuotaUsed, u.ID)
	return err
}

func (s *SQLiteStorage) ListUsers() ([]models.User, error) {
	rows, err := s.db.Query(`SELECT id, sub, email, name, is_admin, quota_limit, quota_used FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Sub, &u.Email, &u.Name, &u.IsAdmin, &u.QuotaLimit, &u.QuotaUsed); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *SQLiteStorage) GetClientKeysByUserID(userID string) ([]models.ClientKey, error) {
	rows, err := s.db.Query(`SELECT key, label, quota_limit, quota_used, credits, active, user_id, status FROM client_keys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]models.ClientKey, 0)
	for rows.Next() {
		var k models.ClientKey
		var uid sql.NullString
		if err := rows.Scan(&k.Key, &k.Label, &k.QuotaLimit, &k.QuotaUsed, &k.Credits, &k.Active, &uid, &k.Status); err != nil {
			return nil, err
		}
		if uid.Valid {
			k.UserID = uid.String
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *SQLiteStorage) RecordUsage(clientKey string, tokens int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Update Client Key
	var userID sql.NullString
	err = tx.QueryRow(`UPDATE client_keys SET quota_used = quota_used + ? WHERE key = ? RETURNING user_id`, tokens, clientKey).Scan(&userID)
	if err != nil {
		return err
	}

	// 2. Update User Global Quota if linked
	if userID.Valid && userID.String != "" {
		_, err = tx.Exec(`UPDATE users SET quota_used = quota_used + ? WHERE id = ?`, tokens, userID.String)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Agent Key Management
func (s *SQLiteStorage) CreateAgentKey(k models.AgentKey) error {
	_, err := s.db.Exec(`INSERT INTO agent_keys (key, label, node_id, balancer_token, user_id, status) VALUES (?, ?, ?, ?, ?, ?)`,
		k.Key, k.Label, k.NodeID, k.BalancerToken, k.UserID, k.Status)
	return err
}

func (s *SQLiteStorage) NormalizeReputation(amount float64) error {
	// Drift up toward 1.0
	_, err := s.db.Exec(`UPDATE agent_keys SET reputation = MIN(1.0, reputation + ?) WHERE reputation < 1.0`, amount)
	if err != nil {
		return err
	}
	// Drift down toward 1.0
	_, err = s.db.Exec(`UPDATE agent_keys SET reputation = MAX(1.0, reputation - ?) WHERE reputation > 1.0`, amount)
	return err
}

func (s *SQLiteStorage) RecordReputation(key string, change float64) error {
	_, err := s.db.Exec(`UPDATE agent_keys SET reputation = MAX(0.1, MIN(5.0, reputation + ?)) WHERE key = ?`,
		change, key)
	return err
}

func (s *SQLiteStorage) GetAgentKey(key string) (models.AgentKey, error) {
	var k models.AgentKey
	var userID sql.NullString
	err := s.db.QueryRow(`SELECT key, label, node_id, balancer_token, credits_earned, reputation, active, user_id, status FROM agent_keys WHERE key = ?`, key).
		Scan(&k.Key, &k.Label, &k.NodeID, &k.BalancerToken, &k.CreditsEarned, &k.Reputation, &k.Active, &userID, &k.Status)
	if err != nil {
		return k, err
	}
	if userID.Valid {
		k.UserID = userID.String
	}
	return k, nil
}

func (s *SQLiteStorage) ListAgentKeys() ([]models.AgentKey, error) {
	rows, err := s.db.Query(`SELECT key, label, node_id, balancer_token, credits_earned, reputation, active, user_id, status FROM agent_keys`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]models.AgentKey, 0)
	for rows.Next() {
		var k models.AgentKey
		var userID sql.NullString
		if err := rows.Scan(&k.Key, &k.Label, &k.NodeID, &k.BalancerToken, &k.CreditsEarned, &k.Reputation, &k.Active, &userID, &k.Status); err != nil {
			return nil, err
		}
		if userID.Valid {
			k.UserID = userID.String
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *SQLiteStorage) GetAgentKeysByUserID(userID string) ([]models.AgentKey, error) {
	rows, err := s.db.Query(`SELECT key, label, node_id, balancer_token, credits_earned, reputation, active, user_id, status FROM agent_keys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]models.AgentKey, 0)
	for rows.Next() {
		var k models.AgentKey
		if err := rows.Scan(&k.Key, &k.Label, &k.NodeID, &k.BalancerToken, &k.CreditsEarned, &k.Reputation, &k.Active, &k.UserID, &k.Status); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *SQLiteStorage) UpdateClientKey(k models.ClientKey) error {
	_, err := s.db.Exec(`UPDATE client_keys SET label = ?, quota_limit = ?, quota_used = ?, credits = ?, active = ?, status = ? WHERE key = ?`,
		k.Label, k.QuotaLimit, k.QuotaUsed, k.Credits, k.Active, k.Status, k.Key)
	return err
}

func (s *SQLiteStorage) DeleteClientKey(key string) error {
	_, err := s.db.Exec(`DELETE FROM client_keys WHERE key = ?`, key)
	return err
}

func (s *SQLiteStorage) RotateAgentKey(oldKey, newKey, newBalancerToken string) (models.AgentKey, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.AgentKey{}, err
	}
	defer tx.Rollback()

	// Copy existing record with new key value
	_, err = tx.Exec(`
		INSERT INTO agent_keys (key, label, node_id, balancer_token, credits_earned, reputation, active, user_id, status)
		SELECT ?, label, node_id, ?, credits_earned, reputation, active, user_id, status
		FROM agent_keys WHERE key = ?`, newKey, newBalancerToken, oldKey)
	if err != nil {
		return models.AgentKey{}, err
	}

	// Remove old record
	_, err = tx.Exec(`DELETE FROM agent_keys WHERE key = ?`, oldKey)
	if err != nil {
		return models.AgentKey{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.AgentKey{}, err
	}

	return s.GetAgentKey(newKey)
}

func (s *SQLiteStorage) DeleteAgentKey(key string) error {
	_, err := s.db.Exec(`DELETE FROM agent_keys WHERE key = ?`, key)
	return err
}

func (s *SQLiteStorage) DeleteUser(id string) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *SQLiteStorage) SetKeyStatus(keyType, key string, status models.KeyStatus, active bool) error {
	table := "client_keys"
	if keyType == "agent" {
		table = "agent_keys"
	}
	query := fmt.Sprintf("UPDATE %s SET status = ?, active = ? WHERE key = ?", table)
	_, err := s.db.Exec(query, status, active, key)
	return err
}

func (s *SQLiteStorage) SetUserModelPolicy(p models.UserModelPolicy) error {
	_, err := s.db.Exec(`INSERT INTO user_model_policies (user_id, model, reward_factor, cost_factor, is_disabled) 
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id, model) DO UPDATE SET 
		reward_factor = excluded.reward_factor, cost_factor = excluded.cost_factor, is_disabled = excluded.is_disabled`,
		p.UserID, p.Model, p.RewardFactor, p.CostFactor, p.Disabled)
	return err
}

func (s *SQLiteStorage) GetUserModelPolicy(userID, model string) (models.UserModelPolicy, error) {
	var p models.UserModelPolicy
	err := s.db.QueryRow(`SELECT user_id, model, reward_factor, cost_factor, is_disabled FROM user_model_policies 
		WHERE user_id = ? AND model = ?`, userID, model).
		Scan(&p.UserID, &p.Model, &p.RewardFactor, &p.CostFactor, &p.Disabled)
	if err != nil {
		// Default policy
		return models.UserModelPolicy{UserID: userID, Model: model, RewardFactor: 1.0, CostFactor: 1.0, Disabled: false}, nil
	}
	return p, nil
}

func (s *SQLiteStorage) ListUserModelPolicies(userID string) ([]models.UserModelPolicy, error) {
	rows, err := s.db.Query(`SELECT user_id, model, reward_factor, cost_factor, is_disabled FROM user_model_policies WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := make([]models.UserModelPolicy, 0)
	for rows.Next() {
		var p models.UserModelPolicy
		if err := rows.Scan(&p.UserID, &p.Model, &p.RewardFactor, &p.CostFactor, &p.Disabled); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, nil
}

func (s *SQLiteStorage) GetModelPolicies() (map[string]map[string]struct{ Banned, Pinned, Persistent bool }, error) {
	rows, err := s.db.Query("SELECT model, node_id, is_banned, is_pinned, is_persistent FROM model_policies")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := make(map[string]map[string]struct{ Banned, Pinned, Persistent bool })
	for rows.Next() {
		var model, nodeID string
		var banned, pinned, persistent bool
		if err := rows.Scan(&model, &nodeID, &banned, &pinned, &persistent); err != nil {
			return nil, err
		}
		if _, ok := policies[model]; !ok {
			policies[model] = make(map[string]struct{ Banned, Pinned, Persistent bool })
		}
		policies[model][nodeID] = struct{ Banned, Pinned, Persistent bool }{Banned: banned, Pinned: pinned, Persistent: persistent}
	}
	return policies, nil
}

func (s *SQLiteStorage) RecordLog(nodeID, level, component, message string) error {
	_, err := s.db.Exec("INSERT INTO logs (timestamp, node_id, level, component, message) VALUES (?, ?, ?, ?, ?)",
		time.Now(), nodeID, level, component, message)
	return err
}

func (s *SQLiteStorage) GetRecentLogs(limit int) ([]struct {
	Timestamp time.Time
	NodeID    string
	Level     string
	Component string
	Message   string
}, error) {
	rows, err := s.db.Query("SELECT timestamp, node_id, level, component, message FROM logs ORDER BY timestamp DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]struct {
		Timestamp time.Time
		NodeID    string
		Level     string
		Component string
		Message   string
	}, 0)
	for rows.Next() {
		var l struct {
			Timestamp time.Time
			NodeID    string
			Level     string
			Component string
			Message   string
		}
		if err := rows.Scan(&l.Timestamp, &l.NodeID, &l.Level, &l.Component, &l.Message); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *SQLiteStorage) SearchLogs(limit int, nodeID, level, query string) ([]models.LogEntry, error) {
	sqlStr := "SELECT timestamp, node_id, level, component, message FROM logs WHERE 1=1"
	var args []interface{}

	if nodeID != "" {
		sqlStr += " AND node_id = ?"
		args = append(args, nodeID)
	}
	if level != "" {
		sqlStr += " AND level = ?"
		args = append(args, level)
	}
	if query != "" {
		sqlStr += " AND message LIKE ?"
		args = append(args, "%"+query+"%")
	}

	sqlStr += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]models.LogEntry, 0)
	for rows.Next() {
		var l models.LogEntry
		if err := rows.Scan(&l.Timestamp, &l.NodeID, &l.Level, &l.Component, &l.Message); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *SQLiteStorage) PruneLogs(keep int) error {
	_, err := s.db.Exec("DELETE FROM logs WHERE id NOT IN (SELECT id FROM logs ORDER BY timestamp DESC LIMIT ?)", keep)
	return err
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

func (s *SQLiteStorage) PruneOldMetrics(days int) error {
	_, err := s.db.Exec("DELETE FROM metrics WHERE timestamp < datetime('now', '-' || ? || ' days')", days)
	return err
}

func (s *SQLiteStorage) UpdateModelVRAM(modelName string, minVRAM uint64) error {
	_, err := s.db.Exec(`
		INSERT INTO model_metadata (model_name, min_vram, updated_at) 
		VALUES (?, ?, ?)
		ON CONFLICT(model_name) DO UPDATE SET 
			min_vram = MAX(min_vram, excluded.min_vram),
			updated_at = excluded.updated_at`,
		modelName, minVRAM, time.Now())
	return err
}

func (s *SQLiteStorage) GetModelVRAM(modelName string) (uint64, error) {
	var minVRAM uint64
	err := s.db.QueryRow("SELECT min_vram FROM model_metadata WHERE model_name = ?", modelName).Scan(&minVRAM)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return minVRAM, nil
}

func (s *SQLiteStorage) RecordMetric(nodeID, modelName string, latency time.Duration, success bool) error {
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := s.db.Exec("INSERT INTO metrics (node_id, model_name, latency, success, timestamp) VALUES (?, ?, ?, ?, ?)",
		nodeID, modelName, latency.Seconds(), successInt, time.Now())
	return err
}

func (s *SQLiteStorage) GetPerformance(nodeID, modelName string) (PerformanceMetric, error) {
	query := `
	SELECT AVG(latency), AVG(success) 
	FROM metrics 
	WHERE node_id = ? AND model_name = ? AND timestamp > ?`

	row := s.db.QueryRow(query, nodeID, modelName, time.Now().Add(-24*time.Hour))

	var avgLatency, successRate sql.NullFloat64
	err := row.Scan(&avgLatency, &successRate)
	if err != nil {
		return PerformanceMetric{}, err
	}

	return PerformanceMetric{
		NodeID:      nodeID,
		ModelName:   modelName,
		AvgLatency:  avgLatency.Float64,
		SuccessRate: successRate.Float64,
		LastUpdated: time.Now(),
	}, nil
}

func (s *SQLiteStorage) GetP90Latency(modelName string) (time.Duration, error) {
	query := `
	SELECT latency FROM metrics 
	WHERE model_name = ? AND success = 1 AND timestamp > ?
	ORDER BY latency ASC LIMIT 100`

	rows, err := s.db.Query(query, modelName, time.Now().Add(-24*time.Hour))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var latencies []float64
	for rows.Next() {
		var l float64
		if err := rows.Scan(&l); err == nil {
			latencies = append(latencies, l)
		}
	}

	if len(latencies) == 0 {
		return 0, nil
	}

	index := int(float64(len(latencies)) * 0.9)
	if index >= len(latencies) {
		index = len(latencies) - 1
	}

	return time.Duration(latencies[index] * float64(time.Second)), nil
}

func (s *SQLiteStorage) PruneMetrics(days int) error {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	_, err := s.db.Exec("DELETE FROM metrics WHERE timestamp <= ?", cutoff)
	return err
}
