package pkg

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/shared/config"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentRegistrationWithToken(t *testing.T) {
	agentToken := "test-agent-token"
	balancerToken := "test-balancer-token"

	// 1. Setup Balancer Config
	bCfg := config.DefaultConfig()
	bCfg.AuthToken = balancerToken // Clients need this
	bCfg.RemoteToken = agentToken  // Agents need this

	b, err := balancer.NewBalancer("localhost:0", ":memory:", bCfg)
	if err != nil {
		t.Fatalf("Failed to create balancer: %v", err)
	}
	balancerSrv := httptest.NewServer(b.SetupRoutes())
	defer balancerSrv.Close()

	// 2. Setup Agent Config (matching what main.go should do now)
	aCfg := config.DefaultConfig()
	aCfg.AuthToken = agentToken   // Balancer needs this to call agent
	aCfg.RemoteToken = agentToken // Agent needs this to call balancer /register

	a := agent.NewAgent("agent-test", "localhost:0", balancerSrv.URL, "http://localhost:11434", aCfg)

	// 3. Try to register
	err = a.Register()
	if err != nil {
		t.Fatalf("Registration failed with correct token: %v", err)
	}

	// 4. Verify failure with WRONG token
	aCfgWrong := config.DefaultConfig()
	aCfgWrong.RemoteToken = "wrong-token"
	aWrong := agent.NewAgent("agent-wrong", "localhost:0", balancerSrv.URL, "http://localhost:11434", aCfgWrong)
	err = aWrong.Register()
	if err == nil {
		t.Fatal("Registration should have failed with wrong token")
	}
	if !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("Expected status 401, got: %v", err)
	}

	// 5. Verify failure with BALANCER_TOKEN (the old bug)
	aCfgBug := config.DefaultConfig()
	aCfgBug.RemoteToken = balancerToken // This was the bug: agent using balancerToken to register
	aBug := agent.NewAgent("agent-bug", "localhost:0", balancerSrv.URL, "http://localhost:11434", aCfgBug)
	err = aBug.Register()
	if err == nil {
		t.Fatal("Registration should have failed with balancer token (the bug scenario)")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("Expected status 403, got: %v", err)
	}
}
