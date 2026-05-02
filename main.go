package main

import (
	"FlakyOllama/pkg/agent"
	aConfig "FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	bConfig "FlakyOllama/pkg/balancer/config"
	"FlakyOllama/pkg/shared/logging"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	bcfg, berr := bConfig.LoadConfig(cfgPath)
	acfg, aerr := aConfig.LoadConfig(cfgPath)
	if aerr != nil {
		logging.Global.Infof("Failed to load config, using defaults: %v", aerr)
		acfg = aConfig.DefaultConfig()
	}
	if berr != nil {
		logging.Global.Infof("Failed to load config, using defaults: %v", berr)
		bcfg = bConfig.DefaultConfig()
	}

	if role == "balancer" {
		if bcfg.AuthToken == "" {
			bcfg.AuthToken = strings.Trim(os.Getenv("BALANCER_TOKEN"), "\"'")
		}
		if bcfg.RemoteToken == "" {
			bcfg.RemoteToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
		}
	} else {
		if acfg.AuthToken == "" {
			acfg.AuthToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
		}
		if acfg.RemoteToken == "" {
			acfg.RemoteToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
		}
	}

	// OIDC Env Mappings
	if os.Getenv("OIDC_ENABLED") == "true" {
		bcfg.OIDC.Enabled = true
	}
	if os.Getenv("OIDC_ENABLE_KEY_APPROVAL") == "true" {
		bcfg.EnableKeyApproval = true
	}
	if v := os.Getenv("OIDC_ISSUER"); v != "" {
		bcfg.OIDC.Issuer = v
	}
	if v := os.Getenv("OIDC_CLIENT_ID"); v != "" {
		bcfg.OIDC.ClientID = v
	}
	if v := os.Getenv("OIDC_CLIENT_SECRET"); v != "" {
		bcfg.OIDC.ClientSecret = v
	}
	if v := os.Getenv("OIDC_REDIRECT_URL"); v != "" {
		bcfg.OIDC.RedirectURL = v
	}
	if v := os.Getenv("OIDC_ADMIN_CLAIM"); v != "" {
		bcfg.OIDC.AdminClaim = v
	}
	if v := os.Getenv("OIDC_ADMIN_VALUE"); v != "" {
		bcfg.OIDC.AdminValue = v
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

		logging.Global.Infof("Initializing balancer on %s with DB %s...", addr, dbPath)
		b, err := balancer.NewBalancer(addr, dbPath, bcfg)
		if err != nil {
			logging.Global.Errorf("CRITICAL: Failed to initialize balancer: %v", err)
			os.Exit(1)
		}
		logging.Global.SetSink(b)

		// Signal handling for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			b.Stop()
		}()

		b.StartBackgroundTasks()
		if err := b.Serve(); err != nil && err != http.ErrServerClosed {
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

		a := agent.NewAgent(id, addr, balancerURL, ollamaURL, acfg)
		logging.Global.SetSink(a)

		// Signal handling for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			a.Stop()
		}()

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

		if err := a.Serve(); err != nil && err != http.ErrServerClosed {
			logging.Global.Errorf("Agent failed: %v", err)
			os.Exit(1)
		}

	default:
		logging.Global.Errorf("Unknown role: %s", role)
		os.Exit(1)
	}
}
