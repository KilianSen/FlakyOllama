package config

import (
	"FlakyOllama/pkg/balancer/models"
	"FlakyOllama/pkg/shared/config"
)

// Config represents the application configuration.
type Config struct {
	PollIntervalMs         int                `json:"poll_interval_ms"`
	Weights                RoutingWeights     `json:"weights"`
	CircuitBreaker         CBConfig           `json:"circuit_breaker"`
	StallTimeoutSec        int                `json:"stall_timeout_sec"`
	EnableHedging          bool               `json:"enable_hedging"`
	HedgingPercentile      float64            `json:"hedging_percentile"`
	MaxQueueDepth          int                `json:"max_queue_depth"`
	TLS                    config.TLSConfig   `json:"tls"`
	AuthToken              string             `json:"auth_token"`               // Token expected from clients
	RemoteToken            string             `json:"remote_token"`             // Token to send to agents/balancer
	EnableModelApproval    bool               `json:"enable_model_approval"`    // FIXME: Currently ignored but most infra is there
	EnableKeyApproval      bool               `json:"enable_key_approval"`      // FIXME: Currently ignored but most infra is there
	ModelRewardFactors     map[string]float64 `json:"model_reward_factors"`     // Agent multipliers
	ModelCostFactors       map[string]float64 `json:"model_cost_factors"`       // Client multipliers
	GlobalRewardMultiplier float64            `json:"global_reward_multiplier"` // Global agent bonus
	GlobalCostMultiplier   float64            `json:"global_cost_multiplier"`   // Global client charge

	// Virtual Models & Pipelines
	VirtualModels map[string]models.VirtualModelConfig `json:"virtual_models"`

	// OIDC Configuration
	OIDC      OIDCConfig `json:"oidc"`
	JWTSecret string     `json:"jwt_secret"`
}

type OIDCConfig struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
	AdminClaim   string `json:"admin_claim"` // Claim to check for admin status (e.g. "groups" or "roles")
	AdminValue   string `json:"admin_value"` // Value in AdminClaim that grants admin status
}

type RoutingWeights struct {
	LocalModelBonus float64 `json:"local_model_bonus"`
	WorkloadPenalty float64 `json:"workload_penalty"`
}

type CBConfig struct {
	ErrorThreshold int `json:"error_threshold"` // Consecutive errors before breaking
	CooloffSec     int `json:"cooloff_sec"`     // Seconds to wait before trying again
}
