package models

import (
	"fmt"
	"time"
)

// NodeState represents the health state of a node.
type NodeState int

const (
	StateHealthy  NodeState = iota
	StateDegraded           // Recent errors, scoring penalty applied
	StateBroken             // Node unresponsive or critical error
)

func (s NodeState) String() string {
	switch s {
	case StateHealthy:
		return "Healthy"
	case StateDegraded:
		return "Degraded"
	case StateBroken:
		return "Broken"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

type NodeStatus struct {
	ID              string      `json:"id"`
	AgentKey        string      `json:"agent_key"`         // The token used to register
	UserID          string      `json:"user_id,omitempty"` // ID of the user owning this node
	IsGlobal        bool        `json:"is_global"`         // True if node is not bound to a user
	BalancerToken   string      `json:"balancer_token"`    // The token expected by the agent
	Address         string      `json:"address"`
	State           NodeState   `json:"state"`
	Tier            string      `json:"tier"`      // "dedicated" or "shared"
	CPUUsage        float64     `json:"cpu_usage"` // Percentage
	CPUCores        int         `json:"cpu_cores"`
	MemoryUsage     float64     `json:"memory_usage"` // Percentage
	MemoryTotal     uint64      `json:"memory_total"` // Bytes
	VRAMTotal       uint64      `json:"vram_total"`   // Bytes
	VRAMUsed        uint64      `json:"vram_used"`    // Bytes
	GPUModel        string      `json:"gpu_model"`
	GPUTemperature  float64     `json:"gpu_temp"`      // Celsius
	ActiveModels    []string    `json:"active_models"` // List of currently loaded models
	LocalModels     []ModelInfo `json:"local_models"`  // Models present on disk
	InputTokens     int64       `json:"input_tokens"`
	OutputTokens    int64       `json:"output_tokens"`
	TokenReward     float64     `json:"token_reward"`
	TokensPerSecond float64     `json:"tokens_per_second"`
	Reputation      float64     `json:"reputation"` // Score 0.1 - 5.0
	Errors          int         `json:"errors"`
	Message         string      `json:"message"`
	Draining        bool        `json:"draining"`
	PrivateNode     bool        `json:"private_node,omitempty"` // True if only the owning user can route here
	HasGPU          bool        `json:"has_gpu"`
	LastSeen        time.Time   `json:"last_seen"`
	CooloffUntil    time.Time   `json:"cooloff_until"`

	// Capability map: per-model performance stats measured by this agent
	ModelCapabilities map[string]ModelCapabilityStats `json:"model_capabilities,omitempty"`
}
