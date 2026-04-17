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
	Tier           string      `json:"tier"` // "dedicated" or "shared"
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
	Model        string                 `json:"model"`
	Prompt       string                 `json:"prompt"`
	Stream       bool                   `json:"stream"`
	Priority     int                    `json:"priority"`
	AllowHedging bool                   `json:"allow_hedging"`
	Options      map[string]interface{} `json:"options"`
}

// ChatRequest is a request for a chat completion.
type ChatRequest struct {
	Model        string                 `json:"model"`
	Messages     []ChatMessage          `json:"messages"`
	Stream       bool                   `json:"stream"`
	Priority     int                    `json:"priority"`
	AllowHedging bool                   `json:"allow_hedging"`
	Options      map[string]interface{} `json:"options"`
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
	Name       string        `json:"name"`
	Model      string        `json:"model"`
	ModifiedAt time.Time     `json:"modified_at"`
	Size       int64         `json:"size"`
	Digest     string        `json:"digest"`
	Details    *ModelDetails `json:"details,omitempty"`
}

type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// RegisterRequest is sent by an Agent to the Balancer.
type RegisterRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Tier    string `json:"tier"`
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

	// Aggregate metrics
	TotalVRAM      uint64  `json:"total_vram"`      // Total VRAM in bytes
	UsedVRAM       uint64  `json:"used_vram"`       // Total used VRAM in bytes
	TotalCPUCores  int     `json:"total_cpu_cores"` // Total CPU cores
	AvgCPUUsage    float64 `json:"avg_cpu_usage"`   // Average CPU usage percentage
	AvgMemoryUsage float64 `json:"avg_mem_usage"`   // Average Memory usage percentage
	UptimeSeconds  int64   `json:"uptime_seconds"`
}

type LogLevel string

const (
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
	LevelDebug LogLevel = "DEBUG"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	NodeID    string    `json:"node_id"`
	Level     LogLevel  `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
}
