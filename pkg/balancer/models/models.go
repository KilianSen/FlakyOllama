package models

import (
	"FlakyOllama/pkg/shared/models"
	"time"
)

type User struct {
	ID                string    `json:"id"`
	Sub               string    `json:"sub"` // OIDC Subject
	Email             string    `json:"email"`
	Name              string    `json:"name"`
	IsAdmin           bool      `json:"is_admin"`
	QuotaLimit        int64     `json:"quota_limit"`
	QuotaUsed         int64     `json:"quota_used"`
	QuotaTier         QuotaTier `json:"quota_tier"`
	DailyQuotaLimit   int64     `json:"daily_quota_limit"`
	WeeklyQuotaLimit  int64     `json:"weekly_quota_limit"`
	MonthlyQuotaLimit int64     `json:"monthly_quota_limit"`
	RoutePreference   string    `json:"route_preference,omitempty"` // "" or "quality" = 429 on quota; "quality_fallback" = fall back to own node
}

type UserWithKey struct {
	User          User      `json:"user"`
	Key           ClientKey `json:"key"`
	AgentEarnings float64   `json:"agent_earnings"`
}

type ProfileResponse struct {
	User       User        `json:"user"`
	ClientKeys []ClientKey `json:"client_keys"`
	AgentKeys  []AgentKey  `json:"agent_keys"`
	QuotaUsage QuotaUsage  `json:"quota_usage"`
}

type UserModelPolicy struct {
	UserID       string  `json:"user_id"`
	Model        string  `json:"model"`
	RewardFactor float64 `json:"reward_factor"` // Multiplier for agent earnings
	CostFactor   float64 `json:"cost_factor"`   // Multiplier for client costs
	Disabled     bool    `json:"disabled"`      // If true, user cannot use this model
}

type AgentKey struct {
	Key             string    `json:"key"`
	Label           string    `json:"label"`
	NodeID          string    `json:"node_id"`        // Node associated with this key
	BalancerToken   string    `json:"balancer_token"` // Token the balancer sends to this agent
	CreditsEarned   float64   `json:"credits_earned"`
	Reputation      float64   `json:"reputation"`
	Active          bool      `json:"active"`
	UserID          string    `json:"user_id,omitempty"` // ID of the user owning this key
	Status          KeyStatus `json:"status"`
	ModelVisibility string    `json:"model_visibility,omitempty"` // "" or "public" = shared, "private" = owner only
}

type KeyStatus string

const (
	KeyStatusPending  KeyStatus = "pending"
	KeyStatusActive   KeyStatus = "active"
	KeyStatusRejected KeyStatus = "rejected"
)

type QuotaTier string

const (
	QuotaTierFree      QuotaTier = "free"
	QuotaTierStandard  QuotaTier = "standard"
	QuotaTierPro       QuotaTier = "pro"
	QuotaTierUnlimited QuotaTier = "unlimited"
	QuotaTierCustom    QuotaTier = "custom"
)

type TierLimits struct {
	Total   int64
	Daily   int64
	Weekly  int64
	Monthly int64
}

var DefaultTiers = map[QuotaTier]TierLimits{
	QuotaTierFree:      {Total: 2_000_000, Daily: 50_000, Weekly: 200_000, Monthly: 500_000},
	QuotaTierStandard:  {Total: -1, Daily: 500_000, Weekly: 2_000_000, Monthly: 6_000_000},
	QuotaTierPro:       {Total: -1, Daily: 2_000_000, Weekly: 8_000_000, Monthly: 20_000_000},
	QuotaTierUnlimited: {Total: -1, Daily: -1, Weekly: -1, Monthly: -1},
	QuotaTierCustom:    {Total: -1, Daily: -1, Weekly: -1, Monthly: -1},
}

type QuotaUsage struct {
	DailyUsed          int64   `json:"daily_used"`
	WeeklyUsed         int64   `json:"weekly_used"`
	MonthlyUsed        int64   `json:"monthly_used"`
	AgentCreditsEarned float64 `json:"agent_credits_earned"`
}

type ClientKey struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	QuotaLimit  int64     `json:"quota_limit"` // Max tokens/credits allowed (-1 for unlimited)
	QuotaUsed   int64     `json:"quota_used"`
	Credits     float64   `json:"credits"` // Balance if using a credit system
	Active      bool      `json:"active"`
	UserID      string    `json:"user_id,omitempty"` // ID of the user owning this key
	Status      KeyStatus `json:"status"`
	ErrorFormat string    `json:"error_format,omitempty"` // "" = default flat, "openai" = OpenAI nested
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
	Model      string      `json:"model"`
	CreatedAt  time.Time   `json:"created_at"`
	Message    ChatMessage `json:"message"`
	Done       bool        `json:"done"`
	TotalDur   int64       `json:"total_duration"`
	PromptEval int         `json:"prompt_eval_count"`
	EvalCount  int         `json:"eval_count"`
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
	Models []models.ModelInfo `json:"models"`
}
