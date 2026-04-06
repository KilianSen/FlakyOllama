package config

import (
	"encoding/json"
	"os"
)

// Config represents the application configuration.
type Config struct {
	KeepAliveDurationSec int     `json:"keep_alive_duration_sec"`
	StaleThreshold       int     `json:"stale_threshold"` // Pending requests per model
	LoadThreshold        float64 `json:"load_threshold"`  // CPU Load percentage
	PollIntervalMs       int     `json:"poll_interval_ms"`
}

func DefaultConfig() *Config {
	return &Config{
		KeepAliveDurationSec: 300, // 5m
		StaleThreshold:       5,
		LoadThreshold:        80.0,
		PollIntervalMs:       100, // 10Hz
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
