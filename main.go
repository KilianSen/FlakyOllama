package main

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"os"
	"strings"
	"time"
)

func main() {
	role := os.Getenv("ROLE")
	if role == "" {
		role = "balancer" // Default role
	}

	nodeID := os.Getenv("AGENT_ID")
	if nodeID == "" {
		if role == "balancer" {
			nodeID = "balancer"
		} else {
			hostname, _ := os.Hostname()
			if hostname != "" {
				nodeID = hostname
			} else {
				nodeID = "agent-1"
			}
		}
	}
	logging.InitGlobal(nodeID, role)

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logging.Global.Infof("Failed to load config, using defaults: %v", err)
		cfg = config.DefaultConfig()
	}

	if role == "balancer" {
		if cfg.AuthToken == "" {
			cfg.AuthToken = strings.Trim(os.Getenv("BALANCER_TOKEN"), "\"'")
		}
		if cfg.RemoteToken == "" {
			cfg.RemoteToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
		}
	} else {
		if cfg.AuthToken == "" {
			cfg.AuthToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
		}
		if cfg.RemoteToken == "" {
			cfg.RemoteToken = strings.Trim(os.Getenv("BALANCER_TOKEN"), "\"'")
		}
	}

	switch role {
	case "balancer":
		addr := os.Getenv("BALANCER_ADDR")
		if addr == "" {
			addr = "0.0.0.0:8080"
		}
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "flakyollama.db"
		}
		b, err := balancer.NewBalancer(addr, dbPath, cfg)
		if err != nil {
			logging.Global.Errorf("Failed to initialize balancer: %v", err)
			os.Exit(1)
		}
		logging.Global.SetSink(b)
		b.StartBackgroundTasks()
		if err := b.Serve(); err != nil {
			logging.Global.Errorf("Balancer failed: %v", err)
			os.Exit(1)
		}

	case "agent":
		id := nodeID
		addr := os.Getenv("AGENT_ADDR")
		if addr == "" {
			addr = "0.0.0.0:8081"
		}
		balancerURL := os.Getenv("BALANCER_URL")
		if balancerURL == "" {
			balancerURL = "http://localhost:8080"
		}
		ollamaURL := os.Getenv("OLLAMA_URL")
		if ollamaURL == "" {
			ollamaURL = "http://localhost:11434"
		}
		dbPath := os.Getenv("DB_PATH")

		a := agent.NewAgent(id, addr, balancerURL, ollamaURL, cfg)
		logging.Global.SetSink(a)

		if dbPath != "" {
			logging.Global.Infof("Agent %s using storage at %s", id, dbPath)
			// Future use: a.SetStorage(dbPath)
		}

		// Background registration (retry until success)
		go func() {
			for {
				if err := a.Register(); err == nil {
					logging.Global.Infof("Agent registered successfully")
					break
				} else {
					logging.Global.Infof("Failed to register, retrying in 5s: %v", err)
					time.Sleep(5 * time.Second)
				}
			}
		}()

		if err := a.Serve(); err != nil {
			logging.Global.Errorf("Agent failed: %v", err)
			os.Exit(1)
		}

	default:
		logging.Global.Errorf("Unknown role: %s", role)
		os.Exit(1)
	}
}
