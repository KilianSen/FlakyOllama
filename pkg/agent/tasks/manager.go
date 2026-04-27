package tasks

import (
	"FlakyOllama/pkg/shared/models"
	"sync"
	"time"
)

type TaskManager struct {
	tasks map[string]*models.AgentTask
	mu    sync.RWMutex
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*models.AgentTask),
	}
}

func (m *TaskManager) AddTask(id, taskType, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[id] = &models.AgentTask{
		ID:        id,
		Type:      taskType,
		Model:     model,
		Status:    models.TaskRunning,
		StartedAt: time.Now(),
	}
}

func (m *TaskManager) CompleteTask(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[id]; ok {
		now := time.Now()
		t.EndedAt = &now
		if err != nil {
			t.Status = models.TaskFailed
			t.Error = err.Error()
		} else {
			t.Status = models.TaskCompleted
		}
	}
}

func (m *TaskManager) ListTasks() []models.AgentTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []models.AgentTask
	for _, t := range m.tasks {
		list = append(list, *t)
	}
	return list
}

func (m *TaskManager) CleanupOldTasks(olderThan time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.tasks {
		if t.Status != models.TaskRunning && time.Since(t.StartedAt) > olderThan {
			delete(m.tasks, id)
		}
	}
}
