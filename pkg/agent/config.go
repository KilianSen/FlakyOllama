package agent

import (
	"FlakyOllama/pkg/shared/config"
	"encoding/json"
	"os"
)

type Config struct {
	TLS         config.TLSConfig `json:"tls"`
	AuthToken   string           `json:"auth_token"`   // Token expected from clients
	RemoteToken string           `json:"remote_token"` // Token to send to agents/balancer
}

func DefaultConfig() *Config {
	return &Config{
		AuthToken:   "flakyollama-agent-secret-change-me-immediately",
		RemoteToken: "flakyollama-agent-secret-change-me-immediately",
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
