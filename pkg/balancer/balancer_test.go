package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBalancer_Route(t *testing.T) {
	b, _ := NewBalancer(":8080", ":memory:", nil)

	// Test case 1: No agents
	_, _, err := b.Route(models.InferenceRequest{Model: "llama2"}, "")
	if err == nil {
		t.Errorf("Expected error when no agents available, got nil")
	}

	// Test case 2: One agent, model not loaded
	b.Agents["agent-1"] = &models.NodeStatus{
		ID:           "agent-1",
		Address:      "localhost:8081",
		CPUUsage:     10.0,
		LastSeen:     time.Now(),
		ActiveModels: []string{},
	}

	id, addr, err := b.Route(models.InferenceRequest{Model: "llama2"}, "")
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
	b.Agents["agent-2"] = &models.NodeStatus{
		ID:           "agent-2",
		Address:      "localhost:8082",
		CPUUsage:     20.0,
		LastSeen:     time.Now(),
		ActiveModels: []string{"llama2"},
	}

	id, addr, err = b.Route(models.InferenceRequest{Model: "llama2"}, "")
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	// Should prefer agent-2 because it has the model loaded, even if CPU is higher
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082), got %s", addr)
	}

	// Test case 4: Two agents, both have model loaded, pick lowest CPU
	// Note: agent-2 currently has session affinity from previous test, so it will still be picked due to stickiness bonus.
	b.Agents["agent-1"].ActiveModels = []string{"llama2"}
	id, addr, err = b.Route(models.InferenceRequest{Model: "llama2"}, "")
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082) due to session stickiness, got %s", addr)
	}
}

func TestBalancer_HandleRegister(t *testing.T) {
	b, _ := NewBalancer(":8080", ":memory:", nil)

	// Mock registration request from an agent with 0.0.0.0
	regBody := `{"id": "agent-0", "address": "0.0.0.0:8081"}`
	req, _ := http.NewRequest("POST", "/register", strings.NewReader(regBody))
	req.RemoteAddr = "192.168.1.50:54321" // Simulated remote address

	rr := httptest.NewRecorder()
	b.NewMux().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	b.Mu.RLock()
	expectedAddr := "192.168.1.50:8081"
	agent, ok := b.Agents[expectedAddr]
	b.Mu.RUnlock()

	if !ok {
		t.Fatalf("Agent at %s not registered", expectedAddr)
	}

	if agent.ID != "agent-0" {
		t.Errorf("Expected ID agent-0, got %s", agent.ID)
	}
}
