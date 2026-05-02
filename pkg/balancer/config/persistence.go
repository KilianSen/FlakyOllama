package config

import (
	"FlakyOllama/pkg/balancer/models"
	"encoding/json"
	"os"
)

func DefaultConfig() *Config {
	return &Config{
		PollIntervalMs: 2000, // 0.5Hz
		Weights: RoutingWeights{
			LocalModelBonus: 2.0,
			WorkloadPenalty: 0.5,
		},
		CircuitBreaker: CBConfig{
			ErrorThreshold: 3,
			CooloffSec:     60,
		},
		StallTimeoutSec:        15,
		EnableHedging:          false,
		HedgingPercentile:      0.95,
		MaxQueueDepth:          100,
		EnableModelApproval:    true,
		EnableKeyApproval:      true,
		GlobalRewardMultiplier: 1.1,
		GlobalCostMultiplier:   1.0,
		ModelRewardFactors:     make(map[string]float64),
		ModelCostFactors:       make(map[string]float64),
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
