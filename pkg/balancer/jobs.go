package balancer

import (
	"fmt"
	"sync"
	"time"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    JobStatus `json:"status"`
	Progress  float64   `json:"progress"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type JobManager struct {
	jobs map[string]*Job
	mu   sync.RWMutex
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*Job),
	}
}

func (jm *JobManager) CreateJob(jobType string) *Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	id := fmt.Sprintf("job_%d", time.Now().UnixNano())
	job := &Job{
		ID:        id,
		Type:      jobType,
		Status:    JobPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jm.jobs[id] = job
	return job
}

func (jm *JobManager) UpdateJob(id string, status JobStatus, progress float64, message string) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if job, ok := jm.jobs[id]; ok {
		job.Status = status
		job.Progress = progress
		job.Message = message
		job.UpdatedAt = time.Now()
	}
}

func (jm *JobManager) GetJob(id string) (Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	if job, ok := jm.jobs[id]; ok {
		return *job, true
	}
	return Job{}, false
}
