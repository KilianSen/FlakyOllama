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
	"sync"
	"time"
)

// Agent handles local telemetry and proxies requests to Ollama.
type Agent struct {
	ID               string
	Address          string
	EffectiveAddress string
	BalancerURL      string
	Monitor          *monitoring.Monitor
	Ollama           *ollama.Client

	// Caching to prevent telemetry storms
	lastStatus     models.NodeStatus
	lastStatusTime time.Time
	statusMu       sync.Mutex
}

func NewAgent(id, address, balancerURL, ollamaURL string) *Agent {
	return &Agent{
		ID:               id,
		Address:          address,
		EffectiveAddress: address, // Default to listening address
		BalancerURL:      balancerURL,
		Monitor:          monitoring.NewMonitor(),
		Ollama:           ollama.NewClient(ollamaURL),
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
	a.EffectiveAddress = address

	tier := os.Getenv("AGENT_TIER")
	if tier == "" {
		tier = "dedicated"
	}

	req := models.RegisterRequest{
		ID:      a.ID,
		Address: a.EffectiveAddress,
		Tier:    tier,
	}
	log.Printf("Registering agent %s with address %s", a.ID, a.EffectiveAddress)
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/telemetry", auth.Middleware(token, a.HandleTelemetry))
	mux.HandleFunc("/inference", auth.Middleware(token, a.HandleInference))
	mux.HandleFunc("/chat", auth.Middleware(token, a.HandleChat))
	mux.HandleFunc("/show", auth.Middleware(token, a.HandleShow))
	mux.HandleFunc("/models/pull", auth.Middleware(token, a.HandlePull))
	mux.HandleFunc("/models/unload", auth.Middleware(token, a.HandleUnload))
	mux.HandleFunc("/models/delete", auth.Middleware(token, a.HandleDelete))

	return mux
}

// Serve starts the HTTP server.
func (a *Agent) Serve() error {
	log.Printf("Agent %s listening on %s", a.ID, a.Address)
	return http.ListenAndServe(a.Address, a.NewMux())
}

func (a *Agent) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()

	// Use cache if it's less than 2 seconds old
	if time.Since(a.lastStatusTime) < 2*time.Second {
		json.NewEncoder(w).Encode(a.lastStatus)
		return
	}

	status, err := a.Monitor.GetStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status.ID = a.ID
	status.Address = a.EffectiveAddress
	status.LastSeen = time.Now()

	// Get currently loaded models
	if active, err := a.Ollama.GetLoadedModels(); err == nil {
		status.ActiveModels = active
	}

	// Get all models on disk
	if local, err := a.Ollama.ListLocalModels(); err == nil {
		status.LocalModels = local
	}

	a.lastStatus = status
	a.lastStatusTime = time.Now()

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
	go func() {
		if err := a.Ollama.Pull(req.Model); err != nil {
			log.Printf("Pull failed for model %s: %v", req.Model, err)
		} else {
			log.Printf("Pull finished for model %s", req.Model)
		}
	}()
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
	log.Printf("Unloading model %s", req.Model)
	if err := a.Ollama.Unload(req.Model); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Agent) HandleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("Deleting model %s from disk", req.Model)
	if err := a.Ollama.Delete(req.Model); err != nil {
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

	log.Printf("Starting inference for model %s", req.Model)

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

	log.Printf("Starting chat completion for model %s", req.Model)

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
