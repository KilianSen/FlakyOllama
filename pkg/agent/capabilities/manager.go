package capabilities

import (
	"FlakyOllama/pkg/shared/models"
	"os"
	"strconv"
	"strings"
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
	MaxErrorRate         float64         `json:"max_error_rate"`          // Reject if error rate > this (e.g., 0.5)
	MaxP95LatencyMs      float64         `json:"max_p95_latency_ms"`      // Reject if P95 latency > this
	MinTPS               float64         `json:"min_tps"`                 // Reject if tokens per second < this
}

// Manager handles local capability policies.
type Manager struct {
	mu     sync.RWMutex
	policy Policy
}

func NewManager() *Manager {
	m := &Manager{
		policy: Policy{
			AllowedModels:      make(map[string]bool),
			DenyList:           make(map[string]bool),
			ModelPriorities:    make(map[string]int),
			MaxCPUThreshold:    90.0,
			MaxMemoryThreshold: 90.0,
			RejectOnHighLoad:   false,
		},
	}
	m.LoadFromEnv()
	return m
}

func (m *Manager) LoadFromEnv() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if v := os.Getenv("AGENT_ALLOWED_MODELS"); v != "" {
		for _, s := range strings.Split(v, ",") {
			m.policy.AllowedModels[strings.TrimSpace(s)] = true
		}
	}

	if v := os.Getenv("AGENT_DENY_MODELS"); v != "" {
		for _, s := range strings.Split(v, ",") {
			m.policy.DenyList[strings.TrimSpace(s)] = true
		}
	}

	if v := os.Getenv("AGENT_MODEL_PRIORITIES"); v != "" {
		// format: model1=10,model2=20
		for _, pair := range strings.Split(v, ",") {
			parts := strings.Split(pair, "=")
			if len(parts) == 2 {
				model := strings.TrimSpace(parts[0])
				prio, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				m.policy.ModelPriorities[model] = prio
			}
		}
	}

	if v := os.Getenv("AGENT_MAX_CPU_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.policy.MaxCPUThreshold = f
		}
	}

	if v := os.Getenv("AGENT_MAX_MEM_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.policy.MaxMemoryThreshold = f
		}
	}

	if v := os.Getenv("AGENT_REJECT_ON_HIGH_LOAD"); v != "" {
		m.policy.RejectOnHighLoad = v == "true"
	}

	if v := os.Getenv("AGENT_MIN_PRIORITY_UNDER_LOAD"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			m.policy.MinPriorityUnderLoad = i
		}
	}

	if v := os.Getenv("AGENT_MAX_ERROR_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.policy.MaxErrorRate = f
		}
	}

	if v := os.Getenv("AGENT_MAX_P95_LATENCY"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.policy.MaxP95LatencyMs = f
		}
	}

	if v := os.Getenv("AGENT_MIN_TPS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.policy.MinTPS = f
		}
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

func (m *Manager) IsModelHealthy(model string, stats models.ModelCapabilityStats) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Only check if we have enough samples (e.g., > 5) to avoid jitter on first few requests
	if stats.RequestCount < 5 {
		return true, ""
	}

	if m.policy.MaxErrorRate > 0 && stats.ErrorRate > m.policy.MaxErrorRate {
		return false, "high error rate"
	}

	if m.policy.MaxP95LatencyMs > 0 && stats.P95DurationMs > m.policy.MaxP95LatencyMs {
		return false, "high p95 latency"
	}

	if m.policy.MinTPS > 0 && stats.AvgTPS < m.policy.MinTPS {
		return false, "low throughput (tps)"
	}

	return true, ""
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
