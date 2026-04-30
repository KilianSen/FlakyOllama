package agent_test

import (
	"FlakyOllama/pkg/agent"
	"FlakyOllama/pkg/agent/capabilities"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentCapabilities(t *testing.T) {
	// 1. Setup mock Ollama
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer mockOllama.Close()

	// 2. Setup Agent
	aCfg := config.DefaultConfig()
	aCfg.RemoteToken = "test-token"
	a := agent.NewAgent("agent-test", "localhost:0", "http://balancer", mockOllama.URL, aCfg)
	mux := a.NewMux()
	agentSrv := httptest.NewServer(mux)
	defer agentSrv.Close()

	client := &http.Client{}

	// 3. Test GET /capabilities (initial state)
	req, _ := http.NewRequest("GET", agentSrv.URL+"/capabilities", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to GET capabilities: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	var policy capabilities.Policy
	json.NewDecoder(resp.Body).Decode(&policy)
	resp.Body.Close()

	if len(policy.AllowedModels) != 0 {
		t.Errorf("Expected 0 allowed models, got %d", len(policy.AllowedModels))
	}

	// 4. Test POST /capabilities (update policy)
	newPolicy := capabilities.Policy{
		AllowedModels: map[string]bool{"allowed-model": true},
		DenyList:      map[string]bool{"denied-model": true},
	}
	body, _ := json.Marshal(newPolicy)
	req, _ = http.NewRequest("POST", agentSrv.URL+"/capabilities", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to POST capabilities: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Test model allowance
	// Allowed model
	reqBody := `{"model": "allowed-model", "prompt": "hi"}`
	req, _ = http.NewRequest("POST", agentSrv.URL+"/api/generate", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for allowed model, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Denied model
	reqBody = `{"model": "denied-model", "prompt": "hi"}`
	req, _ = http.NewRequest("POST", agentSrv.URL+"/api/generate", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status 403 for denied model, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Other model (not in allowed list)
	reqBody = `{"model": "other-model", "prompt": "hi"}`
	req, _ = http.NewRequest("POST", agentSrv.URL+"/api/generate", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed request: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected status 403 for unlisted model when AllowedModels is non-empty, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Test system load rejection
	loadPolicy := capabilities.Policy{
		MaxCPUThreshold:      50.0,
		RejectOnHighLoad:     true,
		MinPriorityUnderLoad: 10,
		ModelPriorities:      map[string]int{"high-priority": 20, "low-priority": 5},
	}
	body, _ = json.Marshal(loadPolicy)
	req, _ = http.NewRequest("POST", agentSrv.URL+"/capabilities", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, _ = client.Do(req)
	resp.Body.Close()

	// Inject high CPU usage
	a.Monitor.InjectStatus(models.NodeStatus{CPUUsage: 80.0})

	// Low priority model should be rejected
	reqBody = `{"model": "low-priority", "prompt": "hi"}`
	req, _ = http.NewRequest("POST", agentSrv.URL+"/api/generate", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 for low priority model under load, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// High priority model should be allowed
	reqBody = `{"model": "high-priority", "prompt": "hi"}`
	req, _ = http.NewRequest("POST", agentSrv.URL+"/api/generate", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for high priority model under load, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
