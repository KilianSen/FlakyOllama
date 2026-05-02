package models

import "time"

type AgentTaskStatus string

const (
	TaskRunning   AgentTaskStatus = "running"
	TaskCompleted AgentTaskStatus = "completed"
	TaskFailed    AgentTaskStatus = "failed"
)

type AgentTask struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // "pull", "push", "create"
	Model     string          `json:"model"`
	Status    AgentTaskStatus `json:"status"`
	Error     string          `json:"error,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
}
