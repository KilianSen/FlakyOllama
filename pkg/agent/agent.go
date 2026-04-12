package agent

import (
	"FlakyOllama/pkg/agent/monitoring"
	"FlakyOllama/pkg/agent/ollama"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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
	Config           *config.Config
	LogCh            chan models.LogEntry

	// Caching to prevent telemetry storms
	lastStatus     models.NodeStatus
	lastStatusTime time.Time
	statusMu       sync.Mutex
}

func NewAgent(id, address, balancerURL, ollamaURL string, cfg *config.Config) *Agent {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &Agent{
		ID:               id,
		Address:          address,
		EffectiveAddress: address, // Default to listening address
		BalancerURL:      balancerURL,
		Monitor:          monitoring.NewMonitor(),
		Ollama:           ollama.NewClient(ollamaURL),
		Config:           cfg,
		LogCh:            make(chan models.LogEntry, 100),
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
	logging.Global.Infof("Registering agent %s with address %s", a.ID, a.EffectiveAddress)
	body, _ := json.Marshal(req)

	agentReq, _ := http.NewRequest("POST", a.BalancerURL+"/register", bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("BALANCER_TOKEN"); token != "" {
		agentReq.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: a.Config.TLS.InsecureSkipVerify,
			},
		},
	}
	resp, err := client.Do(agentReq)
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
	mux.HandleFunc("/embeddings", auth.Middleware(token, a.HandleEmbeddings))
	mux.HandleFunc("/version", auth.Middleware(token, a.HandleVersion))
	mux.HandleFunc("/models/create", auth.Middleware(token, a.HandleCreate))
	mux.HandleFunc("/models/copy", auth.Middleware(token, a.HandleCopy))
	mux.HandleFunc("/models/push", auth.Middleware(token, a.HandlePush))
	mux.HandleFunc("/models/pull", auth.Middleware(token, a.HandlePull))
	mux.HandleFunc("/models/unload", auth.Middleware(token, a.HandleUnload))
	mux.HandleFunc("/models/delete", auth.Middleware(token, a.HandleDelete))

	return mux
}

// Serve starts the HTTP server.
func (a *Agent) Serve() error {
	logging.Global.Infof("Agent %s listening on %s (TLS: %v)", a.ID, a.Address, a.Config.TLS.Enabled)
	go a.StartLogShipper()
	if a.Config.TLS.Enabled {
		return http.ListenAndServeTLS(a.Address, a.Config.TLS.CertFile, a.Config.TLS.KeyFile, a.NewMux())
	}
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
			logging.Global.Infof("Pull failed for model %s: %v", req.Model, err)
		} else {
			logging.Global.Infof("Pull finished for model %s", req.Model)
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
	logging.Global.Infof("Unloading model %s", req.Model)
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
	logging.Global.Infof("Deleting model %s from disk", req.Model)
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

	logging.Global.Infof("Starting inference for model %s", req.Model)

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
		logging.Global.Infof("Inference cancelled by Balancer for model %s", req.Model)
	}
}

func (a *Agent) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logging.Global.Infof("Starting chat completion for model %s", req.Model)

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
		logging.Global.Infof("Chat cancelled by Balancer for model %s", req.Model)
	}
}

func (a *Agent) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream, code, err := a.Ollama.Embeddings(req.Model, req.Input)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, stream)
}

func (a *Agent) HandleVersion(w http.ResponseWriter, r *http.Request) {
	version, err := a.Ollama.Version()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"version": version})
}

func (a *Agent) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Modelfile string `json:"modelfile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream, code, err := a.Ollama.Create(req.Name, req.Modelfile)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, stream)
}

func (a *Agent) HandleCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	code, err := a.Ollama.Copy(req.Source, req.Destination)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Agent) HandlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stream, code, err := a.Ollama.Push(req.Name)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, stream)
}

func (a *Agent) Ship(entry models.LogEntry) {
	select {
	case a.LogCh <- entry:
	default:
	}
}

func (a *Agent) StartLogShipper() {
	scheme := "http"
	if a.Config.TLS.Enabled {
		scheme = "https"
	}
	// Use BalancerURL directly, but ensure scheme is correct
	url := a.BalancerURL
	if strings.Contains(url, "://") {
		// Replace existing scheme
		parts := strings.Split(url, "://")
		url = scheme + "://" + parts[1]
	} else {
		url = scheme + "://" + url
	}
	url = strings.TrimSuffix(url, "/") + "/api/log/collect"

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: a.Config.TLS.InsecureSkipVerify,
			},
		},
		Timeout: 5 * time.Second,
	}

	for {
		entry := <-a.LogCh
		body, _ := json.Marshal(entry)
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		if token := os.Getenv("BALANCER_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
	}
}
