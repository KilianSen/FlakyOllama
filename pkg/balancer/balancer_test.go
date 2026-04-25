package balancer

import (
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/models"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBalancer_Route(t *testing.T) {
	b, _ := NewBalancer(":8080", ":memory:", config.DefaultConfig())

	// Test case 1: No agents
	_, _, err := b.Route(context.Background(), models.InferenceRequest{Model: "llama2"}, "", "")
	if err == nil {
		t.Errorf("Expected error when no agents available, got nil")
	}

	// Test case 2: One agent, model not loaded
	b.State.UpsertNode("localhost:8081", &models.NodeStatus{
		ID:           "agent-1",
		Address:      "localhost:8081",
		CPUUsage:     10.0,
		VRAMTotal:    10 * 1024 * 1024 * 1024,
		LastSeen:     time.Now(),
		ActiveModels: []string{},
	})

	// Wait for actor to process upsert
	time.Sleep(10 * time.Millisecond)

	id, addr, err := b.Route(context.Background(), models.InferenceRequest{Model: "llama2"}, "", "")
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	if addr != "localhost:8081" {
		t.Errorf("Expected route to localhost:8081, got %s", addr)
	}
	if id != "agent-1" {
		t.Errorf("Expected agent-1, got %s", id)
	}

	// Test case 3: Two agents, one has model loaded
	b.State.UpsertNode("localhost:8082", &models.NodeStatus{
		ID:           "agent-2",
		Address:      "localhost:8082",
		CPUUsage:     20.0,
		VRAMTotal:    10 * 1024 * 1024 * 1024,
		LastSeen:     time.Now(),
		ActiveModels: []string{"llama2"},
	})

	time.Sleep(10 * time.Millisecond)

	id, addr, err = b.Route(context.Background(), models.InferenceRequest{Model: "llama2"}, "", "")
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	// Should prefer agent-2 because it has the model loaded, even if CPU is higher
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082), got %s", addr)
	}

	// Test case 4: Two agents, both have model loaded, pick lowest CPU
	// Note: agent-2 currently has session affinity from previous test, so it will still be picked due to stickiness bonus.
	b.State.UpdateNode("localhost:8081", func(n *models.NodeStatus) {
		n.ActiveModels = []string{"llama2"}
	})

	time.Sleep(10 * time.Millisecond)

	id, addr, err = b.Route(context.Background(), models.InferenceRequest{Model: "llama2"}, "", "")
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082) due to session stickiness, got %s", addr)
	}
}

func TestBalancer_HandleRegister(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RemoteToken = "test-remote-token"
	b, _ := NewBalancer(":8080", ":memory:", cfg)

	// Mock registration request from an agent with 0.0.0.0
	regBody := `{"id": "agent-0", "address": "0.0.0.0:8081"}`
	req, _ := http.NewRequest("POST", "/register", strings.NewReader(regBody))
	req.Header.Set("Authorization", "Bearer test-remote-token")
	req.RemoteAddr = "192.168.1.50:54321" // Simulated remote address

	rr := httptest.NewRecorder()
	b.NewMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	time.Sleep(10 * time.Millisecond) // Wait for actor

	expectedAddr := "192.168.1.50:8081"
	snapshot := b.State.GetSnapshot()
	agent, ok := snapshot.Agents[expectedAddr]

	if !ok {
		t.Fatalf("Agent at %s not registered", expectedAddr)
	}

	if agent.ID != "agent-0" {
		t.Errorf("Expected ID agent-0, got %s", agent.ID)
	}
}
