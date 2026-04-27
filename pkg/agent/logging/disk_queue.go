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
	for rows.Next() {
		var id int64
		var payload string
		if err := rows.Scan(&id, &payload); err != nil {
			continue
		}
		var entry models.LogEntry
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			continue
		}
		logs = append(logs, struct {
			ID    int64
			Entry models.LogEntry
		}{id, entry})
	}
	return logs, nil
}

func (q *DiskQueue) DeleteLogs(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	// Simple deletion
	for _, id := range ids {
		_, _ = q.db.Exec("DELETE FROM pending_logs WHERE id = ?", id)
	}
	return nil
}

func (q *DiskQueue) Close() error {
	return q.db.Close()
}
