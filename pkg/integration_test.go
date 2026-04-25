package pkg

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/balancer"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/models"
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
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}{
				Models: []struct {
					Name string `json:"name"`
				}{{Name: "llama2"}},
			})
			return
		}
		if r.URL.Path == "/api/generate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(struct {
				Response string `json:"response"`
				Done     bool   `json:"done"`
			}{
				Response: "Hello from mock Ollama!",
				Done:     true,
			})
			return
		}
	}))
	defer mockOllama.Close()

	// 2. Start Balancer
	bCfg := config.DefaultConfig()
	bCfg.AuthToken = "test-token"
	bCfg.RemoteToken = "test-token"
	bCfg.PollIntervalMs = 100 // Fast polling for test
	b, _ := balancer.NewBalancer("localhost:8080", ":memory:", bCfg)
	balancerSrv := httptest.NewServer(b.NewMux())
	defer balancerSrv.Close()

	// 3. Start Agent
	// Extract port from httptest URL for Agent's "Address"
	balancerURL := balancerSrv.URL
	aCfg := config.DefaultConfig()
	aCfg.AuthToken = "test-token"
	aCfg.RemoteToken = "test-token"
	a := agent.NewAgent("agent-test", "localhost:0", balancerURL, mockOllama.URL, aCfg)
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

	// Wait for agent to be polled successfully (up to 5 seconds)
	deadline := time.Now().Add(5 * time.Second)
	success := false
	for time.Now().Before(deadline) {
		snapshot := b.State.GetSnapshot()
		// Also print current state for debugging if needed
		if agent, ok := snapshot.Agents[a.Address]; ok && len(agent.ActiveModels) > 0 {
			success = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !success {
		t.Fatalf("Agent was not polled successfully within timeout")
	}

	// 6. Send an inference request to the Balancer
	req := models.InferenceRequest{
		Model:  "llama2",
		Prompt: "Tell me a joke.",
	}
	body, _ := json.Marshal(req)
	inferenceReq, _ := http.NewRequest("POST", balancerSrv.URL+"/api/generate", bytes.NewBuffer(body))
	inferenceReq.Header.Set("Content-Type", "application/json")
	inferenceReq.Header.Set("Authorization", "Bearer test-token")

	// Use a client with a timeout to avoid hanging the test forever
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(inferenceReq)
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
