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
	AgentKey       string      `json:"agent_key,omitempty"`
	Address        string      `json:"address"`
	Tier           string      `json:"tier"` // "dedicated" or "shared"
	HasGPU         bool        `json:"has_gpu"`
	CPUUsage       float64     `json:"cpu_usage"` // Percentage
	CPUCores       int         `json:"cpu_cores"`
	MemoryUsage    float64     `json:"memory_usage"` // Percentage
	MemoryTotal    uint64      `json:"memory_total"` // Bytes
	VRAMTotal      uint64      `json:"vram_total"`   // Bytes
	VRAMUsed       uint64      `json:"vram_used"`    // Bytes
	GPUModel       string      `json:"gpu_model"`
	GPUTemperature float64     `json:"gpu_temp"`      // Celsius
	ActiveModels   []string    `json:"active_models"` // List of currently loaded models
	LocalModels    []ModelInfo `json:"local_models"`  // Models available on disk
	LastSeen       time.Time   `json:"last_seen"`
	State          NodeState   `json:"state"`
	Errors         int         `json:"errors"`  // Consecutive errors
	Message        string      `json:"message"` // Status message
	Reputation     float64     `json:"reputation"`
	CooloffUntil   time.Time   `json:"cooloff_until"`
	Draining       bool        `json:"draining"`
	InputTokens    int         `json:"input_tokens"`
	OutputTokens   int         `json:"output_tokens"`
	TokenReward    float64     `json:"token_reward"`
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
	ID       string `json:"id"`
	Address  string `json:"address"`
	Tier     string `json:"tier"`
	HasGPU   bool   `json:"has_gpu"`
	GPUModel string `json:"gpu_model"`
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

	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalReward       float64 `json:"total_reward"`
	TotalCost         float64 `json:"total_cost"`

	ModelRewardFactors map[string]float64 `json:"model_reward_factors"`
	ModelCostFactors   map[string]float64 `json:"model_cost_factors"`

	VirtualModels map[string]VirtualModelConfig `json:"virtual_models"`
	OIDCEnabled   bool                         `json:"oidc_enabled"`

	Performance map[string]struct {
		AvgTTFT     float64 `json:"avg_ttft_ms"`
		AvgDuration float64 `json:"avg_duration_ms"`
		Requests    int     `json:"requests"`
	} `json:"performance"`

	ModelPolicies map[string]map[string]struct{ Banned, Pinned bool } `json:"model_policies"` // model -> node_id -> policy
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

type ModelRequestType string

const (
	RequestPull   ModelRequestType = "pull"
	RequestDelete ModelRequestType = "delete"
	RequestCopy   ModelRequestType = "copy"
)

type ModelRequestStatus string

const (
	StatusPending  ModelRequestStatus = "pending"
	StatusApproved ModelRequestStatus = "approved"
	StatusDeclined ModelRequestStatus = "declined"
)

type ModelRequest struct {
	ID          string             `json:"id"`
	Type        ModelRequestType   `json:"type"`
	Model       string             `json:"model"`
	NodeID      string             `json:"node_id"` // Empty for all nodes
	Status      ModelRequestStatus `json:"status"`
	RequestedAt time.Time          `json:"requested_at"`
	ApprovedAt  *time.Time         `json:"approved_at,omitempty"`
}

type ClientKey struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	QuotaLimit int64   `json:"quota_limit"` // Max tokens/credits allowed (-1 for unlimited)
	QuotaUsed  int64   `json:"quota_used"`
	Credits    float64 `json:"credits"` // Balance if using a credit system
	Active     bool    `json:"active"`
	UserID     string  `json:"user_id,omitempty"` // ID of the user owning this key
}

type User struct {
	ID      string `json:"id"`
	Sub     string `json:"sub"` // OIDC Subject
	Email   string `json:"email"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"is_admin"`
}

type UserModelPolicy struct {
	UserID       string  `json:"user_id"`
	Model        string  `json:"model"`
	RewardFactor float64 `json:"reward_factor"` // Multiplier for agent earnings
	CostFactor   float64 `json:"cost_factor"`   // Multiplier for client costs
	Disabled     bool    `json:"disabled"`      // If true, user cannot use this model
}

type AgentKey struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	NodeID        string  `json:"node_id"` // Node associated with this key
	CreditsEarned float64 `json:"credits_earned"`
	Reputation    float64 `json:"reputation"`
	Active        bool    `json:"active"`
	UserID        string  `json:"user_id,omitempty"` // ID of the user owning this key
}

// VirtualModelConfig defines how a virtual model resolves to real models.
type VirtualModelConfig struct {
	Type       string         `json:"type"`        // "pipeline", "arena", "metric"
	Strategy   string         `json:"strategy"`    // "fastest", "cheapest", "random"
	JudgeModel string         `json:"judge_model"` // If type is judge/pipeline
	Targets    []string       `json:"targets"`     // Real backing models
	Steps      []PipelineStep `json:"steps"`       // Execution flow
}

type PipelineStep struct {
	Action       string            `json:"action"` // "generate", "classify", "check"
	Model        string            `json:"model"`
	SystemPrompt string            `json:"system_prompt"`
	MaxRetries   int               `json:"max_retries"`
	OnSuccess    string            `json:"on_success"` // "next", "return"
	OnFail       string            `json:"on_fail"`    // "retry", "fallback", "error"
	Routes       map[string]string `json:"routes"`     // If action is classify
}
