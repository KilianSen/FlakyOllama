package pkg

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/models"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// 1. Start a mock Ollama server
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ps" {
			json.NewEncoder(w).Encode(struct {
				Models []struct{ Name string `json:"name"` } `json:"models"`
			}{
				Models: []struct{ Name string `json:"name"` }{{Name: "llama2"}},
			})
			return
		}
		if r.URL.Path == "/api/generate" {
			json.NewEncoder(w).Encode(struct {
				Response string `json:"response"`
			}{
				Response: "Hello from mock Ollama!",
			})
			return
		}
	}))
	defer mockOllama.Close()

	// 2. Start Balancer
	b, _ := balancer.NewBalancer("localhost:8080", ":memory:", "", nil)
	balancerSrv := httptest.NewServer(b.NewMux())
	defer balancerSrv.Close()

	// 3. Start Agent
	// Extract port from httptest URL for Agent's "Address"
	balancerURL := balancerSrv.URL
	a := agent.NewAgent("agent-test", "localhost:0", balancerURL, mockOllama.URL)
	agentSrv := httptest.NewServer(a.NewMux())
	defer agentSrv.Close()
	
	// Update agent's address to the actual httptest server address
	a.Address = strings.TrimPrefix(agentSrv.URL, "http://")

	// 4. Register Agent with Balancer
	if err := a.Register(); err != nil {
		t.Fatalf("Failed to register agent: %v", err)
	}

	// 5. Start background tasks (poller, workers, etc.)
	b.StartBackgroundTasks()
	time.Sleep(1000 * time.Millisecond) // Wait for at least one poll

	// 6. Send an inference request to the Balancer
	req := models.InferenceRequest{
		Model:  "llama2",
		Prompt: "Tell me a joke.",
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(balancerSrv.URL+"/api/generate", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to send request to balancer: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Balancer returned status %d", resp.StatusCode)
	}

	var result models.InferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Response != "Hello from mock Ollama!" {
		t.Errorf("Expected 'Hello from mock Ollama!', got '%s'", result.Response)
	}
}
