package main

import (
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/balancer/config"
	"FlakyOllama/pkg/shared/logging"
	"os"
	"strings"
)

func main() {
	logging.InitGlobal("balancer", "core")

	addr := os.Getenv("BALANCER_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "flakyollama.db"
	}

	cfgPath := os.Getenv("CONFIG_PATH")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logging.Global.Infof("Failed to load config, using defaults: %v", err)
		cfg = config.DefaultConfig()
	}

	if cfg.AuthToken == "" {
		cfg.AuthToken = strings.Trim(os.Getenv("BALANCER_TOKEN"), "\"'")
	}
	if cfg.RemoteToken == "" {
		cfg.RemoteToken = strings.Trim(os.Getenv("AGENT_TOKEN"), "\"'")
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
}
