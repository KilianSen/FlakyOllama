package config

import (
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"os"
)

// Config represents the application configuration.
type Config struct {
	KeepAliveDurationSec   int                `json:"keep_alive_duration_sec"`
	StaleThreshold         int                `json:"stale_threshold"` // Pending requests per model
	LoadThreshold          float64            `json:"load_threshold"`  // CPU Load percentage
	PollIntervalMs         int                `json:"poll_interval_ms"`
	Weights                RoutingWeights     `json:"weights"`
	CircuitBreaker         CBConfig           `json:"circuit_breaker"`
	StallTimeoutSec        int                `json:"stall_timeout_sec"`
	EnableHedging          bool               `json:"enable_hedging"`
	HedgingPercentile      float64            `json:"hedging_percentile"`
	MaxQueueDepth          int                `json:"max_queue_depth"`
	TLS                    TLSConfig          `json:"tls"`
	AuthToken              string             `json:"auth_token"`   // Token expected from clients
	RemoteToken            string             `json:"remote_token"` // Token to send to agents/balancer
	EnableModelApproval    bool               `json:"enable_model_approval"`
	EnableKeyApproval      bool               `json:"enable_key_approval"`
	ModelRewardFactors     map[string]float64 `json:"model_reward_factors"`     // Agent multipliers
	ModelCostFactors       map[string]float64 `json:"model_cost_factors"`       // Client multipliers
	GlobalRewardMultiplier float64            `json:"global_reward_multiplier"` // Global agent bonus
	GlobalCostMultiplier   float64            `json:"global_cost_multiplier"`   // Global client charge

	// Agent-side capping
	MaxVRAMAllocated uint64 `json:"max_vram_allocated"` // 0 for unlimited
	MaxCPUAllocated  int    `json:"max_cpu_allocated"`  // 0 for unlimited

	// Auto-scaling
	EnableAutoScaling  bool `json:"enable_auto_scaling"`
	AutoScaleThreshold int  `json:"auto_scale_threshold"` // Queue depth per model

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

type TLSConfig struct {
	Enabled            bool   `json:"enabled"`
	CertFile           string `json:"cert_file"`
	KeyFile            string `json:"key_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"` // Useful for self-signed certs
}

type RoutingWeights struct {
	CPULoadWeight     float64 `json:"cpu_load_weight"`
	LatencyWeight     float64 `json:"latency_weight"`
	SuccessRateWeight float64 `json:"success_rate_weight"`
	LoadedModelBonus  float64 `json:"loaded_model_bonus"`
	LocalModelBonus   float64 `json:"local_model_bonus"`
	WorkloadPenalty   float64 `json:"workload_penalty"`
}

type CBConfig struct {
	ErrorThreshold int `json:"error_threshold"` // Consecutive errors before breaking
	CooloffSec     int `json:"cooloff_sec"`     // Seconds to wait before trying again
}

func DefaultConfig() *Config {
	return &Config{
		KeepAliveDurationSec: 300, // 5m
		StaleThreshold:       5,
		LoadThreshold:        80.0,
		PollIntervalMs:       2000, // 0.5Hz
		Weights: RoutingWeights{
			CPULoadWeight:     1.0,
			LatencyWeight:     1.0,
			SuccessRateWeight: 1.0,
			LoadedModelBonus:  5.0,
			LocalModelBonus:   2.0,
			WorkloadPenalty:   0.5,
		},
		CircuitBreaker: CBConfig{
			ErrorThreshold: 3,
			CooloffSec:     60,
		},
		StallTimeoutSec:        15,
		EnableHedging:          true,
		HedgingPercentile:      0.95,
		MaxQueueDepth:          100,
		EnableModelApproval:    true,
		EnableKeyApproval:      false,
		GlobalRewardMultiplier: 1.1,
		GlobalCostMultiplier:   1.0,
		ModelRewardFactors:     make(map[string]float64),
		ModelCostFactors:       make(map[string]float64),
		EnableAutoScaling:      true,
		AutoScaleThreshold:     5,
		MaxVRAMAllocated:       12 * 1024 * 1024 * 1024, // 12GB
		MaxCPUAllocated:        8,
		VirtualModels: map[string]models.VirtualModelConfig{
			"smart-fastest": {
				Type:     "metric",
				Strategy: "fastest",
				Targets:  []string{"llama3:8b", "mistral:7b", "gemma2:2b"},
			},
			"smart-reliable": {
				Type:     "metric",
				Strategy: "most_reliable",
				Targets:  []string{"llama3:8b", "mistral:7b"},
			},
			"smart-cheap": {
				Type:     "metric",
				Strategy: "cheapest",
				Targets:  []string{"llama3.2:1b", "gemma2:2b", "phi3:latest"},
			},
			"auto-grader": {
				Type:       "pipeline",
				JudgeModel: "llama3:70b",
				Targets:    []string{"llama3:8b"},
			},
		},
		JWTSecret: "flakyollama-secret-change-me-immediately",
	}
}

func LoadConfig(path string) (*Config, error) {
	c := DefaultConfig()
	if path == "" {
		return c, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(c)
	return c, err
}

func (c *Config) SaveConfig(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}
