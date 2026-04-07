package agent

import (
	"FlakyOllama/pkg/auth"
	"FlakyOllama/pkg/models"
	"FlakyOllama/pkg/monitoring"
	"FlakyOllama/pkg/ollama"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Agent handles local telemetry and proxies requests to Ollama.
type Agent struct {
	ID          string
	Address     string
	BalancerURL string
	Monitor     *monitoring.Monitor
	Ollama      *ollama.Client
}

func NewAgent(id, address, balancerURL, ollamaURL string) *Agent {
	return &Agent{
		ID:          id,
		Address:     address,
		BalancerURL: balancerURL,
		Monitor:     monitoring.NewMonitor(),
		Ollama:      ollama.NewClient(ollamaURL),
	}
}

// Register registers the agent with the balancer.
func (a *Agent) Register() error {
	address := a.Address
	if strings.HasPrefix(address, "0.0.0.0:") || strings.HasPrefix(address, ":") {
		// If listening on all interfaces, register with the hostname
		// In Docker, the hostname is usually the container ID which is resolvable
		hostname, err := os.Hostname()
		if err == nil {
			_, port, _ := net.SplitHostPort(address)
			address = net.JoinHostPort(hostname, port)
		}
	}

	req := models.RegisterRequest{
		ID:      a.ID,
		Address: address,
	}
	log.Printf("Registering agent %s with address %s", a.ID, address)
	body, _ := json.Marshal(req)

	agentReq, _ := http.NewRequest("POST", a.BalancerURL+"/register", bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("BALANCER_TOKEN"); token != "" {
		agentReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(agentReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to register with balancer: status %d", resp.StatusCode)
	}

	return nil
}

// NewMux returns a mux with the agent's handlers registered.
func (a *Agent) NewMux() *http.ServeMux {
	token := os.Getenv("AGENT_TOKEN")
	mux := http.NewServeMux()

	mux.HandleFunc("/telemetry", auth.Middleware(token, a.HandleTelemetry))
	mux.HandleFunc("/inference", auth.Middleware(token, a.HandleInference))
	mux.HandleFunc("/chat", auth.Middleware(token, a.HandleChat))
	mux.HandleFunc("/show", auth.Middleware(token, a.HandleShow))
	mux.HandleFunc("/models/pull", auth.Middleware(token, a.HandlePull))
	mux.HandleFunc("/models/unload", auth.Middleware(token, a.HandleUnload))

	return mux
}

// Serve starts the HTTP server.
func (a *Agent) Serve() error {
	log.Printf("Agent %s listening on %s", a.ID, a.Address)
	return http.ListenAndServe(a.Address, a.NewMux())
}

func (a *Agent) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	status, err := a.Monitor.GetStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status.ID = a.ID
	status.Address = a.Address
	status.LastSeen = time.Now()

	models, err := a.Ollama.GetLoadedModels()
	if err == nil {
		status.ActiveModels = models
	}

	json.NewEncoder(w).Encode(status)
}

func (a *Agent) HandlePull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go a.Ollama.Pull(req.Model) // Async pull
	w.WriteHeader(http.StatusAccepted)
}

func (a *Agent) HandleUnload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.Ollama.Unload(req.Model); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Agent) HandleShow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := a.Ollama.Show(req.Model)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (a *Agent) HandleInference(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Propagation of context for cancellation
	stream, code, err := a.Ollama.GenerateStream(req)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Create a pipe to detect client disconnects during streaming
	done := make(chan struct{})
	go func() {
		io.Copy(w, stream)
		close(done)
	}()

	select {
	case <-done:
		// Completed successfully
	case <-r.Context().Done():
		log.Printf("Inference cancelled by Balancer for model %s", req.Model)
	}
}

func (a *Agent) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream, code, err := a.Ollama.ChatStream(req)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	done := make(chan struct{})
	go func() {
		io.Copy(w, stream)
		close(done)
	}()

	select {
	case <-done:
	case <-r.Context().Done():
		log.Printf("Chat cancelled by Balancer for model %s", req.Model)
	}
}
