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
	ID             string      `json:"id"`
	AgentKey       string      `json:"agent_key"`         // The token used to register
	UserID         string      `json:"user_id,omitempty"` // ID of the user owning this node
	IsGlobal       bool        `json:"is_global"`         // True if node is not bound to a user
	BalancerToken  string      `json:"balancer_token"`    // The token expected by the agent
	Address        string      `json:"address"`
	State          NodeState   `json:"state"`
	Tier           string      `json:"tier"`      // "dedicated" or "shared"
	CPUUsage       float64     `json:"cpu_usage"` // Percentage
	CPUCores       int         `json:"cpu_cores"`
	MemoryUsage    float64     `json:"memory_usage"` // Percentage
	MemoryTotal    uint64      `json:"memory_total"` // Bytes
	VRAMTotal      uint64      `json:"vram_total"`   // Bytes
	VRAMUsed       uint64      `json:"vram_used"`    // Bytes
	GPUModel       string      `json:"gpu_model"`
	GPUTemperature float64     `json:"gpu_temp"`      // Celsius
	ActiveModels   []string    `json:"active_models"` // List of currently loaded models
	LocalModels    []ModelInfo `json:"local_models"`  // Models present on disk
	InputTokens    int64       `json:"input_tokens"`
	OutputTokens   int64       `json:"output_tokens"`
	TokenReward    float64     `json:"token_reward"`
	Reputation     float64     `json:"reputation"` // Score 0.1 - 5.0
	Errors         int         `json:"errors"`
	Message        string      `json:"message"`
	Draining       bool        `json:"draining"`
	HasGPU         bool        `json:"has_gpu"`
	LastSeen       time.Time   `json:"last_seen"`
	CooloffUntil   time.Time   `json:"cooloff_until"`
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

type InferenceRequest struct {
	Model        string                 `json:"model"`
	Prompt       string                 `json:"prompt"`
	Options      map[string]interface{} `json:"options"`
	Stream       bool                   `json:"stream"`
	Priority     int                    `json:"priority"`
	AllowHedging bool                   `json:"allow_hedging"`
}

type InferenceResponse struct {
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"created_at"`
	Response    string    `json:"response"`
	Done        bool      `json:"done"`
	TotalDur    int64     `json:"total_duration"`
	LoadDur     int64     `json:"load_duration"`
	SampleCount int       `json:"sample_count"`
	SampleDur   int64     `json:"sample_duration"`
	PromptCount int       `json:"prompt_eval_count"`
	PromptDur   int64     `json:"prompt_eval_duration"`
	EvalCount   int       `json:"eval_count"`
	EvalDur     int64     `json:"eval_duration"`
}

type ChatRequest struct {
	Model        string                 `json:"model"`
	Messages     []ChatMessage          `json:"messages"`
	Options      map[string]interface{} `json:"options"`
	Stream       bool                   `json:"stream"`
	Priority     int                    `json:"priority"`
	AllowHedging bool                   `json:"allow_hedging"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Model     string      `json:"model"`
	CreatedAt time.Time   `json:"created_at"`
	Message   ChatMessage `json:"message"`
	Done      bool        `json:"done"`
	TotalDur  int64       `json:"total_duration"`
}

type OllamaEmbeddingsRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type OllamaEmbeddingsResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

// RegisterRequest is sent by an Agent to the Balancer.
type RegisterRequest struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Tier     string `json:"tier"`
	HasGPU   bool   `json:"has_gpu"`
	GPUModel string `json:"gpu_model"`
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
	StatusPending    ModelRequestStatus = "pending"
	StatusApproved   ModelRequestStatus = "approved"
	StatusProcessing ModelRequestStatus = "processing"
	StatusCompleted  ModelRequestStatus = "completed"
	StatusRejected   ModelRequestStatus = "rejected"
	StatusFailed     ModelRequestStatus = "failed"
)

type ModelRequest struct {
	ID          string             `json:"id"`
	Type        ModelRequestType   `json:"type"`
	Model       string             `json:"model"`
	NodeID      string             `json:"node_id"` // Empty for all nodes
	Status      ModelRequestStatus `json:"status"`
	AgentTaskID string             `json:"agent_task_id,omitempty"`
	RequestedAt time.Time          `json:"requested_at"`
	ApprovedAt  *time.Time         `json:"approved_at,omitempty"`
}

type KeyStatus string

const (
	KeyStatusPending  KeyStatus = "pending"
	KeyStatusActive   KeyStatus = "active"
	KeyStatusRejected KeyStatus = "rejected"
)

type ClientKey struct {
	Key        string    `json:"key"`
	Label      string    `json:"label"`
	QuotaLimit int64     `json:"quota_limit"` // Max tokens/credits allowed (-1 for unlimited)
	QuotaUsed  int64     `json:"quota_used"`
	Credits    float64   `json:"credits"` // Balance if using a credit system
	Active     bool      `json:"active"`
	UserID     string    `json:"user_id,omitempty"` // ID of the user owning this key
	Status     KeyStatus `json:"status"`
}

type User struct {
	ID         string `json:"id"`
	Sub        string `json:"sub"` // OIDC Subject
	Email      string `json:"email"`
	Name       string `json:"name"`
	IsAdmin    bool   `json:"is_admin"`
	QuotaLimit int64  `json:"quota_limit"`
	QuotaUsed  int64  `json:"quota_used"`
}

type UserWithKey struct {
	User User      `json:"user"`
	Key  ClientKey `json:"key"`
}

type ProfileResponse struct {
	User       User        `json:"user"`
	ClientKeys []ClientKey `json:"client_keys"`
	AgentKeys  []AgentKey  `json:"agent_keys"`
}

type UserModelPolicy struct {
	UserID       string  `json:"user_id"`
	Model        string  `json:"model"`
	RewardFactor float64 `json:"reward_factor"` // Multiplier for agent earnings
	CostFactor   float64 `json:"cost_factor"`   // Multiplier for client costs
	Disabled     bool    `json:"disabled"`      // If true, user cannot use this model
}

type ClusterStatus struct {
	Nodes             map[string]*NodeStatus `json:"nodes"`
	ActiveWorkloads   int                    `json:"active_workloads"`
	AvgCPUUsage       float64                `json:"avg_cpu_usage"`
	AvgMemUsage       float64                `json:"avg_mem_usage"`
	PendingRequests   map[string]int         `json:"pending_requests"`
	AllModels         []string               `json:"all_models"`
	TotalInputTokens  int64                  `json:"total_input_tokens"`
	TotalOutputTokens int64                  `json:"total_output_tokens"`
	TotalReward       float64                `json:"total_reward"`
	TotalCost         float64                `json:"total_cost"`
	UptimeSeconds     int64                  `json:"uptime_seconds"`
	TotalVRAM         uint64                 `json:"total_vram"`
	UsedVRAM          uint64                 `json:"used_vram"`
	Performance       map[string]struct {
		AvgTTFT     float64 `json:"avg_ttft_ms"`
		AvgDuration float64 `json:"avg_duration_ms"`
		Requests    int     `json:"requests"`
	} `json:"performance"`

	ModelRewardFactors     map[string]float64 `json:"model_reward_factors"`
	ModelCostFactors       map[string]float64 `json:"model_cost_factors"`
	GlobalRewardMultiplier float64            `json:"global_reward_multiplier"`
	GlobalCostMultiplier   float64            `json:"global_cost_multiplier"`
	OIDCEnabled            bool               `json:"oidc_enabled"`
	QueueDepth             int                `json:"queue_depth"`
	NodeWorkloads          map[string]int     `json:"node_workloads"`
}

type Catalog struct {
	GlobalRewardMultiplier float64 `json:"global_reward_multiplier"`
	GlobalCostMultiplier   float64 `json:"global_cost_multiplier"`
	Models                 []struct {
		Name         string  `json:"name"`
		RewardFactor float64 `json:"reward_factor"`
		CostFactor   float64 `json:"cost_factor"`
	} `json:"models"`
}

type QueuedRequestInfo struct {
	ID          string    `json:"id"`
	Model       string    `json:"model"`
	Priority    int       `json:"priority"`
	ClientIP    string    `json:"client_ip"`
	ContextHash string    `json:"context_hash"`
	QueuedAt    time.Time `json:"queued_at"`
}

type AgentKey struct {
	Key           string    `json:"key"`
	Label         string    `json:"label"`
	NodeID        string    `json:"node_id"`        // Node associated with this key
	BalancerToken string    `json:"balancer_token"` // Token the balancer sends to this agent
	CreditsEarned float64   `json:"credits_earned"`
	Reputation    float64   `json:"reputation"`
	Active        bool      `json:"active"`
	UserID        string    `json:"user_id,omitempty"` // ID of the user owning this key
	Status        KeyStatus `json:"status"`
}

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
type TelemetryResponse struct {
	Status           string   `json:"status"`
	PersistentModels []string `json:"persistent_models,omitempty"`
}
