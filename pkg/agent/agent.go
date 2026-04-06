package agent

import (
	"FlakyOllama/pkg/models"
	"FlakyOllama/pkg/monitoring"
	"FlakyOllama/pkg/ollama"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	req := models.RegisterRequest{
		ID:      a.ID,
		Address: a.Address,
	}
	body, _ := json.Marshal(req)
	
	resp, err := http.Post(a.BalancerURL+"/register", "application/json", bytes.NewBuffer(body))
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
	mux := http.NewServeMux()
	mux.HandleFunc("/telemetry", a.HandleTelemetry)
	mux.HandleFunc("/inference", a.HandleInference)
	mux.HandleFunc("/models/pull", a.HandlePull)
	mux.HandleFunc("/models/unload", a.HandleUnload)
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
	var req struct{ Model string `json:"model"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go a.Ollama.Pull(req.Model) // Async pull
	w.WriteHeader(http.StatusAccepted)
}

func (a *Agent) HandleUnload(w http.ResponseWriter, r *http.Request) {
	var req struct{ Model string `json:"model"` }
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

func (a *Agent) HandleInference(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	resp, err := a.Ollama.Generate(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	
	json.NewEncoder(w).Encode(resp)
}
