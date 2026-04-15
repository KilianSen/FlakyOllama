package jobs

import (
	"sync"
	"time"
)

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

type Job struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    JobStatus `json:"status"`
	Message   string    `json:"message,omitempty"`
	Progress  float64   `json:"progress"` // 0.0 to 1.0
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

func (jm *JobManager) CreateJob(id, jobType string) *Job {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	job := &Job{
		ID:        id,
		Type:      jobType,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jm.jobs[id] = job
	return job
}

func (jm *JobManager) GetJob(id string) (*Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	job, ok := jm.jobs[id]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid mutation outside of manager
	jobCopy := *job
	return &jobCopy, true
}

func (jm *JobManager) UpdateJob(id string, update func(*Job)) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if job, ok := jm.jobs[id]; ok {
		update(job)
		job.UpdatedAt = time.Now()
	}
}

func (jm *JobManager) ListJobs() []*Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	list := make([]*Job, 0, len(jm.jobs))
	for _, job := range jm.jobs {
		jobCopy := *job
		list = append(list, &jobCopy)
	}
	return list
}

func (jm *JobManager) CleanupOldJobs(maxAge time.Duration) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	now := time.Now()
	for id, job := range jm.jobs {
		if now.Sub(job.UpdatedAt) > maxAge {
			delete(jm.jobs, id)
		}
	}
}
