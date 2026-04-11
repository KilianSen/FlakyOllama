package models

import "time"

// NodeState represents the health state of a node.
type NodeState int

const (
	StateHealthy  NodeState = iota
	StateDegraded           // Recent errors, scoring penalty applied
	StateBroken             // Too many errors, excluded from routing
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
		return "Unknown"
	}
}

// NodeStatus represents the current state of an Agent node.
type NodeStatus struct {
	ID             string      `json:"id"`
	Address        string      `json:"address"`
	HasGPU         bool        `json:"has_gpu"`
	CPUUsage       float64     `json:"cpu_usage"` // Percentage
	CPUCores       int         `json:"cpu_cores"`
	MemoryUsage    float64     `json:"memory_usage"` // Percentage
	VRAMTotal      uint64      `json:"vram_total"`   // Bytes
	VRAMUsed       uint64      `json:"vram_used"`    // Bytes
	GPUModel       string      `json:"gpu_model"`
	GPUTemperature float64     `json:"gpu_temp"`      // Celsius
	ActiveModels   []string    `json:"active_models"` // List of currently loaded models
	LocalModels    []ModelInfo `json:"local_models"`  // Models available on disk
	LastSeen       time.Time   `json:"last_seen"`
	State          NodeState   `json:"state"`
	Errors         int         `json:"errors"` // Consecutive errors
	CooloffUntil   time.Time   `json:"cooloff_until"`
	Draining       bool        `json:"draining"`
}

// ModelRequirement defines the hardware needs for a model.
type ModelRequirement struct {
	MinVRAM uint64 `json:"min_vram"` // Bytes
}

// InferenceRequest is a simplified request for an LLM task.
type InferenceRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options"`
}

// ChatRequest is a request for a chat completion.
type ChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ChatMessage          `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options"`
}

// ChatMessage represents a single message in a chat history.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InferenceResponse is the result of an inference task.
type InferenceResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// TagsResponse represents the response from /api/tags
type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

type ModelInfo struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
}

// RegisterRequest is sent by an Agent to the Balancer.
type RegisterRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// ClusterStatus represents the complete state of the cluster for the dashboard.
type ClusterStatus struct {
	Nodes           map[string]*NodeStatus `json:"nodes"`
	PendingRequests map[string]int         `json:"pending_requests"`
	InProgressPulls map[string]time.Time   `json:"in_progress_pulls"`
	NodeWorkloads   map[string]int         `json:"node_workloads"`
	QueueDepth      int                    `json:"queue_depth"`
	ActiveWorkloads int                    `json:"active_workloads"`
	AllModels       []string               `json:"all_models"`
}
