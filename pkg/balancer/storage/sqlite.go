package storage

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"time"
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
	db, err := sql.Open("sqlite3", path)
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
	}
	for _, q := range queries {
		if _, err = db.Exec(q); err != nil {
			return nil, err
		}
	}

	return &SQLiteStorage{db: db}, nil
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
