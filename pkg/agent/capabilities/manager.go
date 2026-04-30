package capabilities

import (
	"sync"
)

// Policy defines local rules for the agent.
type Policy struct {
	AllowedModels        map[string]bool `json:"allowed_models"`       // If empty, all models are allowed unless in DenyList
	DenyList             map[string]bool `json:"deny_list"`            // Models specifically blocked
	ModelPriorities      map[string]int  `json:"model_priorities"`     // Priority levels (higher = more important)
	MaxCPUThreshold      float64         `json:"max_cpu_threshold"`    // Reject requests if CPU usage > this
	MaxMemoryThreshold   float64         `json:"max_memory_threshold"` // Reject requests if Memory usage > this
	RejectOnHighLoad     bool            `json:"reject_on_high_load"`
	MinPriorityUnderLoad int             `json:"min_priority_under_load"` // Minimum priority required when under high load
}

// Manager handles local capability policies.
type Manager struct {
	mu     sync.RWMutex
	policy Policy
}

func NewManager() *Manager {
	return &Manager{
		policy: Policy{
			AllowedModels:      make(map[string]bool),
			DenyList:           make(map[string]bool),
			ModelPriorities:    make(map[string]int),
			MaxCPUThreshold:    90.0,
			MaxMemoryThreshold: 90.0,
			RejectOnHighLoad:   false,
		},
	}
}

func (m *Manager) GetPolicy() Policy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policy
}

func (m *Manager) UpdatePolicy(p Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.AllowedModels == nil {
		p.AllowedModels = make(map[string]bool)
	}
	if p.DenyList == nil {
		p.DenyList = make(map[string]bool)
	}
	if p.ModelPriorities == nil {
		p.ModelPriorities = make(map[string]int)
	}
	m.policy = p
}

func (m *Manager) IsModelAllowed(model string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.policy.DenyList[model] {
		return false
	}

	if len(m.policy.AllowedModels) > 0 {
		return m.policy.AllowedModels[model]
	}

	return true
}

func (m *Manager) GetPriority(model string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policy.ModelPriorities[model]
}

func (m *Manager) ShouldRejectLoad(cpuUsage, memUsage float64, modelPriority int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.policy.RejectOnHighLoad {
		return false
	}

	isUnderHighLoad := false
	if m.policy.MaxCPUThreshold > 0 && cpuUsage > m.policy.MaxCPUThreshold {
		isUnderHighLoad = true
	}
	if m.policy.MaxMemoryThreshold > 0 && memUsage > m.policy.MaxMemoryThreshold {
		isUnderHighLoad = true
	}

	if isUnderHighLoad {
		if modelPriority < m.policy.MinPriorityUnderLoad {
			return true
		}
	}

	return false
}
