package logging

import (
	"FlakyOllama/pkg/shared/models"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DiskQueue struct {
	dbPath string
	db     *sql.DB
	mu     sync.Mutex
}

func NewDiskQueue(path string) (*DiskQueue, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Create table for logs
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS pending_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		payload TEXT,
		created_at DATETIME
	)`)
	if err != nil {
		return nil, err
	}

	return &DiskQueue{
		dbPath: path,
		db:     db,
	}, nil
}

func (q *DiskQueue) Ship(entry models.LogEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()

	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}

	_, _ = q.db.Exec("INSERT INTO pending_logs (payload, created_at) VALUES (?, ?)",
		string(payload), time.Now())
}

func (q *DiskQueue) FetchLogs(limit int) ([]struct {
	ID    int64
	Entry models.LogEntry
}, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	rows, err := q.db.Query("SELECT id, payload FROM pending_logs ORDER BY id ASC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []struct {
		ID    int64
		Entry models.LogEntry
	}
	var corruptIDs []int64
	for rows.Next() {
		var id int64
		var payload string
		if err := rows.Scan(&id, &payload); err != nil {
			corruptIDs = append(corruptIDs, id)
			continue
		}
		var entry models.LogEntry
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			corruptIDs = append(corruptIDs, id)
			continue
		}
		logs = append(logs, struct {
			ID    int64
			Entry models.LogEntry
		}{id, entry})
	}
	rows.Close()

	if len(corruptIDs) > 0 {
		tx, err := q.db.Begin()
		if err == nil {
			stmt, _ := tx.Prepare("DELETE FROM pending_logs WHERE id = ?")
			for _, id := range corruptIDs {
				_, _ = stmt.Exec(id)
			}
			stmt.Close()
			tx.Commit()
		}
	}

	return logs, nil
}

func (q *DiskQueue) DeleteLogs(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("DELETE FROM pending_logs WHERE id = ?")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		_, _ = stmt.Exec(id)
	}
	return tx.Commit()
}

func (q *DiskQueue) Close() error {
	return q.db.Close()
}
