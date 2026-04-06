package balancer

import (
	"FlakyOllama/pkg/models"
	"testing"
	"time"
)

func TestBalancer_Route(t *testing.T) {
	b, _ := NewBalancer(":8080", ":memory:", nil)
	
	// Test case 1: No agents
	_, _, err := b.Route(models.InferenceRequest{Model: "llama2"})
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
	
	id, addr, err := b.Route(models.InferenceRequest{Model: "llama2"})
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
	
	id, addr, err = b.Route(models.InferenceRequest{Model: "llama2"})
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	// Should prefer agent-2 because it has the model loaded, even if CPU is higher
	if addr != "localhost:8082" {
		t.Errorf("Expected route to agent-2 (localhost:8082), got %s", addr)
	}

	// Test case 4: Two agents, both have model loaded, pick lowest CPU
	b.Agents["agent-1"].ActiveModels = []string{"llama2"}
	id, addr, err = b.Route(models.InferenceRequest{Model: "llama2"})
	if err != nil {
		t.Fatalf("Failed to route: %v", err)
	}
	if addr != "localhost:8081" {
		t.Errorf("Expected route to agent-1 (localhost:8081) due to lower CPU, got %s", addr)
	}
}
