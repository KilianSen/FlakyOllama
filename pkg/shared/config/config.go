package config

import (
	"encoding/json"
	"os"
)

// Config represents the application configuration.
type Config struct {
	KeepAliveDurationSec int            `json:"keep_alive_duration_sec"`
	StaleThreshold       int            `json:"stale_threshold"` // Pending requests per model
	LoadThreshold        float64        `json:"load_threshold"`  // CPU Load percentage
	PollIntervalMs       int            `json:"poll_interval_ms"`
	Weights              RoutingWeights `json:"weights"`
	CircuitBreaker       CBConfig       `json:"circuit_breaker"`
	StallTimeoutSec      int            `json:"stall_timeout_sec"`
	EnableHedging        bool           `json:"enable_hedging"`
	HedgingPercentile    float64        `json:"hedging_percentile"`
	MaxQueueDepth        int            `json:"max_queue_depth"`
	TLS                  TLSConfig      `json:"tls"`
	AuthToken            string         `json:"auth_token"`   // Token expected from clients
	RemoteToken          string         `json:"remote_token"` // Token to send to agents/balancer
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
		StallTimeoutSec:   15,
		EnableHedging:     true,
		HedgingPercentile: 0.95,
		MaxQueueDepth:     100,
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
