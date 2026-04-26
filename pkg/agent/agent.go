package agent

import (
	"FlakyOllama/pkg/agent/monitoring"
	"FlakyOllama/pkg/agent/ollama"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
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
	AgentKey         string // The secret token for registration
	Address          string // Listening address (e.g. 0.0.0.0:8081)
	EffectiveAddress string // Publicly reachable address
	BalancerURL      string
	Monitor          *monitoring.Monitor
	Ollama           *ollama.Client
	Config           *config.Config

	lastStatus     models.NodeStatus
	lastStatusTime time.Time
	statusMu       sync.Mutex

	LogCh  chan models.LogEntry
	stopCh chan struct{}

	httpServer *http.Server
}

func NewAgent(id, address, balancerURL, ollamaURL string, cfg *config.Config) *Agent {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	key := os.Getenv("AGENT_KEY")
	if key == "" {
		key = os.Getenv("AGENT_TOKEN")
	}
	if key == "" && cfg != nil {
		key = cfg.RemoteToken
	}

	return &Agent{
		ID:               id,
		AgentKey:         key,
		Address:          address,
		EffectiveAddress: address, // Default to listening address
		BalancerURL:      balancerURL,
		Monitor:          monitoring.NewMonitor(),
		Ollama:           ollama.NewClient(ollamaURL),
		Config:           cfg,
		LogCh:            make(chan models.LogEntry, 100),
		stopCh:           make(chan struct{}),
	}
}

// Register registers the agent with the balancer.
func (a *Agent) Register() error {
	address := a.Address
	if strings.HasPrefix(address, "0.0.0.0:") || strings.HasPrefix(address, ":") {
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

	status, _ := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)

	req := models.RegisterRequest{
		ID:       a.ID,
		Address:  a.EffectiveAddress,
		Tier:     tier,
		HasGPU:   status.HasGPU,
		GPUModel: status.GPUModel,
	}
	logging.Global.Infof("Registering agent %s with address %s [GPU: %v (%s)]", a.ID, a.EffectiveAddress, req.HasGPU, req.GPUModel)
	body, _ := json.Marshal(req)

	agentReq, _ := http.NewRequest("POST", a.BalancerURL+"/register", bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")

	// Auth: Prefer AgentKey, fallback to RemoteToken
	token := a.AgentKey
	if token == "" {
		token = a.Config.RemoteToken
	}
	if token != "" {
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
	token := a.Config.AuthToken
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/telemetry", auth.Middleware([]string{token}, nil, a.HandleTelemetry))
	mux.HandleFunc("/inference", auth.Middleware([]string{token}, nil, a.HandleInference))
	mux.HandleFunc("/chat", auth.Middleware([]string{token}, nil, a.HandleChat))

	// OpenAI Compatibility Routes (forward to Ollama native /v1)
	mux.HandleFunc("/v1/", auth.Middleware([]string{token}, nil, a.HandleV1Proxy))

	mux.HandleFunc("/show", auth.Middleware([]string{token}, nil, a.HandleShow))
	mux.HandleFunc("/embeddings", auth.Middleware([]string{token}, nil, a.HandleEmbeddings))
	mux.HandleFunc("/version", auth.Middleware([]string{token}, nil, a.HandleVersion))
	mux.HandleFunc("/models/create", auth.Middleware([]string{token}, nil, a.HandleCreate))
	mux.HandleFunc("/models/copy", auth.Middleware([]string{token}, nil, a.HandleCopy))
	mux.HandleFunc("/models/push", auth.Middleware([]string{token}, nil, a.HandlePush))
	mux.HandleFunc("/models/pull", auth.Middleware([]string{token}, nil, a.HandlePull))
	mux.HandleFunc("/models/unload", auth.Middleware([]string{token}, nil, a.HandleUnload))
	mux.HandleFunc("/models/delete", auth.Middleware([]string{token}, nil, a.HandleDelete))

	return mux
}

// Serve starts the HTTP server.
func (a *Agent) Serve() error {
	logging.Global.Infof("Agent %s listening on %s (TLS: %v)", a.ID, a.Address, a.Config.TLS.Enabled)

	a.httpServer = &http.Server{
		Addr:    a.Address,
		Handler: a.NewMux(),
	}

	// Start background tasks
	go a.StartLogShipper()
	go a.StartRegistrationLoop()

	if a.Config.TLS.Enabled {
		return a.httpServer.ListenAndServeTLS(a.Config.TLS.CertFile, a.Config.TLS.KeyFile)
	}
	return a.httpServer.ListenAndServe()
}

func (a *Agent) Stop() {
	logging.Global.Infof("Agent %s shutting down...", a.ID)
	close(a.stopCh)
	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.httpServer.Shutdown(ctx)
	}
}

// StartRegistrationLoop ensures the agent stays registered even if the balancer restarts.
func (a *Agent) StartRegistrationLoop() {
	// Register immediately on start
	if err := a.Register(); err != nil {
		logging.Global.Errorf("Initial registration failed: %v", err)
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.Register(); err != nil {
				logging.Global.Debugf("Periodic re-registration failed: %v", err)
			}
		case <-a.stopCh:
			return
		}
	}
}

func (a *Agent) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()

	// Use cache if it's less than 2 seconds old
	if time.Since(a.lastStatusTime) < 2*time.Second {
		json.NewEncoder(w).Encode(a.lastStatus)
		return
	}

	status, err := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)
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

func (a *Agent) HandleV1Proxy(w http.ResponseWriter, r *http.Request) {
	// Transparently forward OpenAI requests to Ollama's native /v1 endpoint
	// This ensures SSE and OpenAI formatting are preserved
	url := a.Ollama.BaseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		url += "?" + r.URL.RawQuery
	}

	req, _ := http.NewRequest(r.Method, url, r.Body)
	for k, v := range r.Header {
		if k != "Authorization" && k != "Host" {
			req.Header[k] = v
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
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
		logging.Global.Infof("Inference completed for model %s", req.Model)
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

func (a *Agent) StartLogShipper() {
	scheme := "http"
	if a.Config.TLS.Enabled {
		scheme = "https"
	}
	url := a.BalancerURL
	if strings.Contains(url, "://") {
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
		select {
		case entry := <-a.LogCh:
			body, _ := json.Marshal(entry)
			req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			token := a.AgentKey
			if token == "" {
				token = a.Config.RemoteToken
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}

			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
		case <-a.stopCh:
			return
		}
	}
}

// LogSink implementation
func (a *Agent) Ship(entry models.LogEntry) {
	select {
	case a.LogCh <- entry:
	default:
	}
}
