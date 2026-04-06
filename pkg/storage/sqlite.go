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
	query := `
	CREATE TABLE IF NOT EXISTS metrics (
		node_id TEXT,
		model_name TEXT,
		latency REAL,
		success INTEGER,
		timestamp DATETIME,
		PRIMARY KEY (node_id, model_name, timestamp)
	);`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}

	return &SQLiteStorage{db: db}, nil
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
