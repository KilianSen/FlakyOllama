package balancer

import (
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBalancer_SelectAgent(t *testing.T) {
	b, _ := NewBalancer(":8080", ":memory:", config.DefaultConfig())

	// Test case 1: No agents
	_, err := b.SelectAgent("llama2", "")
	if err == nil {
		t.Errorf("Expected error when no agents available, got nil")
	}

	// Test case 2: One agent, model not loaded
	b.State.Do(func(s *ClusterState) {
		s.Agents["localhost:8081"] = &models.NodeStatus{
			ID:           "agent-1",
			Address:      "localhost:8081",
			CPUUsage:     10.0,
			VRAMTotal:    10 * 1024 * 1024 * 1024,
			LastSeen:     time.Now(),
			ActiveModels: []string{},
			Reputation:   1.0,
			State:        models.StateHealthy,
		}
	})

	// Wait for actor to process
	time.Sleep(10 * time.Millisecond)

	addr, err := b.SelectAgent("llama2", "")
	if err != nil {
		t.Fatalf("Failed to select agent: %v", err)
	}
	if addr != "localhost:8081" {
		t.Errorf("Expected route to localhost:8081, got %s", addr)
	}

	// Test case 3: Two agents, one has model loaded
	b.State.Do(func(s *ClusterState) {
		s.Agents["localhost:8082"] = &models.NodeStatus{
			ID:           "agent-2",
			Address:      "localhost:8082",
			CPUUsage:     20.0,
			VRAMTotal:    10 * 1024 * 1024 * 1024,
			LastSeen:     time.Now(),
			ActiveModels: []string{"llama2"},
			Reputation:   1.0,
			State:        models.StateHealthy,
		}
	})

	time.Sleep(10 * time.Millisecond)

	addr, err = b.SelectAgent("llama2", "")
	if err != nil {
		t.Fatalf("Failed to select agent: %v", err)
	}
	// Should prefer agent-2 because it has the model loaded, even if CPU is higher
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082), got %s", addr)
	}

	// Test case 4: Two agents, both have model loaded, pick lowest CPU
	b.State.Do(func(s *ClusterState) {
		s.Agents["localhost:8081"].ActiveModels = []string{"llama2"}
		s.Agents["localhost:8081"].LocalModels = []models.ModelInfo{{Name: "llama2"}}
		s.Agents["localhost:8082"].LocalModels = []models.ModelInfo{{Name: "llama2"}}
	})

	time.Sleep(10 * time.Millisecond)

	addr, err = b.SelectAgent("llama2", "")
	if err != nil {
		t.Fatalf("Failed to select agent: %v", err)
	}
	// agent-1 has 10% CPU, agent-2 has 20% CPU.
	if addr != "localhost:8081" {
		t.Errorf("Expected route to agent-1 (localhost:8081) due to lower CPU, got %s", addr)
	}

	// Test case 5: Budget-Aware Scoring
	// agent-1: Reputation 1.0
	// agent-2: Reputation 5.0
	// User with high CostFactor should prefer agent-2
	b.State.Do(func(s *ClusterState) {
		s.Agents["localhost:8081"].Reputation = 1.0
		s.Agents["localhost:8082"].Reputation = 5.0
		s.Agents["localhost:8081"].CPUUsage = 10.0
		s.Agents["localhost:8082"].CPUUsage = 10.0
	})
	b.Storage.SetUserModelPolicy(models.UserModelPolicy{
		UserID:     "rich_user",
		Model:      "llama2",
		CostFactor: 2.0,
	})

	time.Sleep(10 * time.Millisecond)

	addr, err = b.SelectAgent("llama2", "rich_user")
	if err != nil {
		t.Fatalf("Failed to select agent for rich_user: %v", err)
	}
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (high reputation) for rich_user, got %s", addr)
	}
}

func TestBalancer_HandleRegister(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RemoteToken = "test-remote-token"
	b, _ := NewBalancer(":8080", ":memory:", cfg)

	// Mock registration request from an agent
	regBody := `{"id": "agent-0", "address": "192.168.1.50:8081"}`
	req, _ := http.NewRequest("POST", "/register", strings.NewReader(regBody))
	req.Header.Set("Authorization", "Bearer test-remote-token")

	rr := httptest.NewRecorder()
	b.SetupRoutes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	time.Sleep(10 * time.Millisecond) // Wait for actor

	expectedAddr := "192.168.1.50:8081"
	snapshot := b.State.GetSnapshot()
	agent, ok := snapshot.Agents[expectedAddr]

	if !ok {
		t.Fatalf("Agent at %s not registered. Snapshot: %+v", expectedAddr, snapshot.Agents)
	}

	if agent.ID != "agent-0" {
		t.Errorf("Expected ID agent-0, got %s", agent.ID)
	}
}
