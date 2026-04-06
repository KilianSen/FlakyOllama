package main

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/config"
	"log"
	"os"
	"time"
)

func main() {
	role := os.Getenv("ROLE")
	if role == "" {
		role = "balancer" // Default role
	}

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = config.DefaultConfig()
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
			log.Fatalf("Failed to initialize balancer: %v", err)
		}
		b.StartBackgroundTasks()
		if err := b.Serve(); err != nil {
			log.Fatalf("Balancer failed: %v", err)
		}

	case "agent":
		id := os.Getenv("AGENT_ID")
		if id == "" {
			id = "agent-1"
		}
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

		a := agent.NewAgent(id, addr, balancerURL, ollamaURL)
		
		// Background registration (retry until success)
		go func() {
			for {
				if err := a.Register(); err == nil {
					log.Printf("Agent registered successfully")
					break
				} else {
					log.Printf("Failed to register, retrying in 5s: %v", err)
					time.Sleep(5 * time.Second)
				}
			}
		}()

		if err := a.Serve(); err != nil {
			log.Fatalf("Agent failed: %v", err)
		}

	default:
		log.Fatalf("Unknown role: %s", role)
	}
}
