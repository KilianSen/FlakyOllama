package agent

import (
	"FlakyOllama/pkg/agent/capabilities"
	agentLogging "FlakyOllama/pkg/agent/logging"
	"FlakyOllama/pkg/agent/monitoring"
	"FlakyOllama/pkg/agent/ollama"
	"FlakyOllama/pkg/agent/tasks"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	sharedLog "FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// metricsResponseWriter wraps http.ResponseWriter to capture first-byte time and
// the tail of the response body so we can parse Ollama's final eval chunk.
type metricsResponseWriter struct {
	http.ResponseWriter
	startTime  time.Time
	ttft       time.Duration
	firstWrite bool
	// ring buffer keeping last 2 KB of response for final-chunk parsing
	tail    [2048]byte
	tailPos int
	tailLen int
	status  int
}

func (m *metricsResponseWriter) WriteHeader(code int) {
	m.status = code
	m.ResponseWriter.WriteHeader(code)
}

func (m *metricsResponseWriter) Write(b []byte) (int, error) {
	if !m.firstWrite && len(b) > 0 {
		m.ttft = time.Since(m.startTime)
		m.firstWrite = true
	}
	for _, c := range b {
		m.tail[m.tailPos%len(m.tail)] = c
		m.tailPos++
		if m.tailLen < len(m.tail) {
			m.tailLen++
		}
	}
	return m.ResponseWriter.Write(b)
}

func (m *metricsResponseWriter) Flush() {
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// tailString returns the buffered tail as a string (handles wrap-around).
func (m *metricsResponseWriter) tailString() string {
	if m.tailLen < len(m.tail) {
		return string(m.tail[:m.tailLen])
	}
	start := m.tailPos % len(m.tail)
	return string(m.tail[start:]) + string(m.tail[:start])
}

// ollamaFinalChunk is what we look for in the tail to extract token counts.
type ollamaFinalChunk struct {
	Done            bool   `json:"done"`
	Model           string `json:"model"`
	EvalCount       int64  `json:"eval_count"`
	EvalDuration    int64  `json:"eval_duration"` // nanoseconds
	PromptEvalCount int64  `json:"prompt_eval_count"`
}

// Agent handles local telemetry and proxies requests to Ollama.
type Agent struct {
	ID               string
	AgentKey         string
	BalancerToken    string
	Address          string
	EffectiveAddress string
	BalancerURL      string
	Monitor          *monitoring.Monitor
	Metrics          *monitoring.ModelMetricsTracker
	Ollama           *ollama.Client
	Config           *config.Config
	Tasks            *tasks.TaskManager
	Capabilities     *capabilities.Manager
	Logs             *agentLogging.DiskQueue

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	httpClient *http.Client
	httpServer *http.Server
	proxy      *httputil.ReverseProxy

	persistentModels []string
	persistentMu     sync.RWMutex
}

func NewAgent(id, address, balancerURL, ollamaURL string, cfg *config.Config) *Agent {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		agentToken = os.Getenv("AGENT_KEY")
	}

	balancerToken := os.Getenv("BALANCER_TOKEN")
	if balancerToken == "" && cfg != nil {
		balancerToken = cfg.RemoteToken
	}

	// If no agent token is provided, assume global agent and use balancer token
	if agentToken == "" {
		agentToken = balancerToken
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize DiskQueue for logs
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("agent_logs_%s.db", id))
	dq, err := agentLogging.NewDiskQueue(dbPath)
	if err != nil {
		sharedLog.Global.Errorf("Failed to initialize disk queue: %v", err)
	}

	// Initialize Reverse Proxy
	target, _ := url.Parse(ollamaURL)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			sharedLog.Global.Errorf("Proxy error for %s: %v", r.URL.Path, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{
				"error":  "Ollama unreachable",
				"detail": err.Error(),
			})
		},
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.InsecureSkipVerify = cfg.TLS.InsecureSkipVerify

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	a := &Agent{
		ID:               id,
		AgentKey:         agentToken,
		BalancerToken:    balancerToken,
		Address:          address,
		EffectiveAddress: address,
		BalancerURL:      balancerURL,
		Monitor:          monitoring.NewMonitor(),
		Metrics:          monitoring.NewModelMetricsTracker(),
		Ollama:           ollama.NewClient(ollamaURL),
		Config:           cfg,
		Tasks:            tasks.NewTaskManager(),
		Capabilities:     capabilities.NewManager(),
		Logs:             dq,
		ctx:              ctx,
		cancel:           cancel,
		httpClient:       httpClient,
		proxy:            proxy,
	}

	// Set disk queue as the sink for the global logger if it exists
	if dq != nil {
		sharedLog.Global.SetSink(dq)
	}

	return a
}

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

	addressOverride := os.Getenv("AGENT_ADDRESS")
	if addressOverride != "" {
		a.EffectiveAddress = addressOverride
	}

	status, _ := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)

	req := models.RegisterRequest{
		ID:       a.ID,
		Address:  a.EffectiveAddress,
		Tier:     tier,
		HasGPU:   status.HasGPU,
		GPUModel: status.GPUModel,
	}
	sharedLog.Global.Infof("Registering agent %s with address %s [GPU: %v (%s)]", a.ID, a.EffectiveAddress, req.HasGPU, req.GPUModel)

	body, _ := json.Marshal(req)
	agentReq, _ := http.NewRequestWithContext(a.ctx, "POST", a.BalancerURL+"/api/v1/nodes/register", bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")

	if a.AgentKey != "" {
		agentReq.Header.Set("Authorization", "Bearer "+a.AgentKey)
	}

	resp, err := a.httpClient.Do(agentReq)
	if err != nil {
		sharedLog.Global.Errorf("Registration request failed for agent %s: %v", a.ID, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		sharedLog.Global.Errorf("Balancer rejected registration for agent %s: status %d, body: %s", a.ID, resp.StatusCode, string(bytes.TrimSpace(bodyBytes)))
		return fmt.Errorf("failed to register with balancer: status %d", resp.StatusCode)
	}

	var result models.TelemetryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		a.persistentMu.Lock()
		a.persistentModels = result.PersistentModels
		a.persistentMu.Unlock()
	} else {
		sharedLog.Global.Warnf("Registration response decode failed for agent %s: %v", a.ID, err)
	}

	sharedLog.Global.Infof("Successfully registered agent %s (persistent models: %d)", a.ID, len(a.persistentModels))
	return nil
}
func (a *Agent) NewMux() *http.ServeMux {
	masterTokens := []string{}
	if a.BalancerToken != "" {
		masterTokens = append(masterTokens, a.BalancerToken)
	}

	adminToken := os.Getenv("AGENT_AUTH_TOKEN")
	if adminToken != "" {
		masterTokens = append(masterTokens, adminToken)
	} else if a.AgentKey != a.BalancerToken && a.AgentKey != "" {
		masterTokens = append(masterTokens, a.AgentKey)
	}

	tokenDisable := os.Getenv("AGENT_AUTH_TOKEN_DISABLE")
	if tokenDisable == "true" {
		sharedLog.Global.Warnf("****************************************************************")
		sharedLog.Global.Warnf("SECURITY WARNING: AGENT_AUTH_TOKEN_DISABLE is true!")
		sharedLog.Global.Warnf("Anyone with access to this port can control Ollama on this agent.")
		sharedLog.Global.Warnf("****************************************************************")
		masterTokens = []string{""}
	}

	if len(masterTokens) == 0 {
		sharedLog.Global.Warnf("Agent auth: no tokens configured — all requests to agent endpoints will be REJECTED (set BALANCER_TOKEN or AGENT_AUTH_TOKEN_DISABLE=true)")
	} else {
		masked := make([]string, len(masterTokens))
		for i, t := range masterTokens {
			if len(t) > 8 {
				masked[i] = t[:8] + "••••"
			} else {
				masked[i] = "••••"
			}
		}
		sharedLog.Global.Infof("Agent auth: accepting %d token(s): %v", len(masterTokens), masked)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/telemetry", auth.Middleware(masterTokens, nil, a.HandleTelemetry))
	mux.HandleFunc("/capabilities", auth.Middleware(masterTokens, nil, a.HandleCapabilities))
	mux.HandleFunc("/tasks", auth.Middleware(masterTokens, nil, a.HandleTasks))

	// Proxy routes using ReverseProxy
	mux.HandleFunc("/v1/", auth.Middleware(masterTokens, nil, a.genericProxy))
	mux.HandleFunc("/api/generate", auth.Middleware(masterTokens, nil, a.metricsProxy))
	mux.HandleFunc("/api/chat", auth.Middleware(masterTokens, nil, a.metricsProxy))
	mux.HandleFunc("/api/embeddings", auth.Middleware(masterTokens, nil, a.genericProxy))

	// Direct handlers for more control
	mux.HandleFunc("/show", auth.Middleware(masterTokens, nil, a.HandleShow))
	mux.HandleFunc("/version", auth.Middleware(masterTokens, nil, a.HandleVersion))

	// Async Task handlers
	mux.HandleFunc("/api/models/pull", auth.Middleware(masterTokens, nil, a.HandlePull))
	mux.HandleFunc("/api/models/unload", auth.Middleware(masterTokens, nil, a.HandleUnload))
	mux.HandleFunc("/api/models/delete", auth.Middleware(masterTokens, nil, a.HandleDelete))
	mux.HandleFunc("/api/models/create", auth.Middleware(masterTokens, nil, a.HandleCreate))
	mux.HandleFunc("/api/models/copy", auth.Middleware(masterTokens, nil, a.HandleCopy))
	mux.HandleFunc("/api/models/push", auth.Middleware(masterTokens, nil, a.HandlePush))

	return mux
}

func (a *Agent) Serve() error {
	sharedLog.Global.Infof("Agent %s listening on %s (TLS: %v)", a.ID, a.Address, a.Config.TLS.Enabled)

	a.httpServer = &http.Server{
		Addr:    a.Address,
		Handler: a.NewMux(),
	}

	// Start background tasks
	a.Monitor.Start(a.ctx)

	a.wg.Add(3)
	go a.StartLogShipper()
	go a.StartRegistrationLoop()
	go a.StartMaintenanceLoop()

	if a.Config.TLS.Enabled {
		return a.httpServer.ListenAndServeTLS(a.Config.TLS.CertFile, a.Config.TLS.KeyFile)
	}
	return a.httpServer.ListenAndServe()
}

func (a *Agent) StartMaintenanceLoop() {
	defer a.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.persistentMu.RLock()
			modelsToKeep := make([]string, len(a.persistentModels))
			copy(modelsToKeep, a.persistentModels)
			a.persistentMu.RUnlock()

			a.Tasks.CleanupOldTasks(24 * time.Hour)

			if len(modelsToKeep) == 0 {
				continue
			}

			ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
			active, err := a.Ollama.GetLoadedModels(ctx)
			cancel()

			if err != nil {
				sharedLog.Global.Warnf("Maintenance: Failed to get loaded models: %v", err)
				continue
			}

			activeMap := make(map[string]bool)
			for _, m := range active {
				activeMap[m] = true
			}

			for _, m := range modelsToKeep {
				if !activeMap[m] {
					sharedLog.Global.Infof("Maintenance: Pre-warming persistent model %s", m)
					ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
					if err := a.Ollama.LoadPersistent(ctx, m); err != nil {
						sharedLog.Global.Errorf("Maintenance: Failed to load persistent model %s: %v", m, err)
					}
					cancel()
				}
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) Stop() {
	sharedLog.Global.Infof("Agent %s shutting down...", a.ID)
	a.cancel()
	a.Monitor.Stop()

	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.httpServer.Shutdown(ctx)
	}

	a.wg.Wait()
	if a.Logs != nil {
		a.Logs.Close()
	}
}

func (a *Agent) StartRegistrationLoop() {
	defer a.wg.Done()
	if err := a.Register(); err != nil {
		sharedLog.Global.Errorf("Initial registration failed: %v", err)
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.Register(); err != nil {
				sharedLog.Global.Debugf("Periodic re-registration failed: %v", err)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) checkCapabilities(w http.ResponseWriter, r *http.Request, model string) bool {
	// 1. Check if model is allowed
	if model != "" && !a.Capabilities.IsModelAllowed(model) {
		sharedLog.Global.Warnf("Rejecting request: model %s is disallowed by local policy", model)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "model disallowed by agent local policy"})
		return false
	}

	// 2. Check system load
	status, err := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)
	if err == nil {
		priority := a.Capabilities.GetPriority(model)
		if a.Capabilities.ShouldRejectLoad(status.CPUUsage, status.MemoryUsage, priority) {
			sharedLog.Global.Warnf("Rejecting request for %s: high system load (CPU: %.1f%%, Mem: %.1f%%, Priority: %d)", model, status.CPUUsage, status.MemoryUsage, priority)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "agent too busy",
				"cpu":   fmt.Sprintf("%.1f%%", status.CPUUsage),
				"mem":   fmt.Sprintf("%.1f%%", status.MemoryUsage),
			})
			return false
		}
	}

	return true
}

// metricsProxy wraps the reverse proxy for /api/generate and /api/chat,
// capturing TTFT, duration, and token counts from Ollama's final eval chunk.
func (a *Agent) metricsProxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Peek at the request body to extract the model name, then restore it.
	var model string
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		if err == nil {
			var peek struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(bodyBytes, &peek)
			model = peek.Model
		}
	}

	if !a.checkCapabilities(w, r, model) {
		return
	}

	mrw := &metricsResponseWriter{
		ResponseWriter: w,
		startTime:      start,
	}

	a.proxy.ServeHTTP(mrw, r)

	totalMs := float64(time.Since(start).Milliseconds())
	ttftMs := float64(mrw.ttft.Milliseconds())
	isError := mrw.status >= 400

	sample := monitoring.RequestSample{
		Model:      model,
		TTFTMs:     ttftMs,
		DurationMs: totalMs,
		Error:      isError,
	}

	if !isError {
		// Parse Ollama's final streaming chunk from the tail buffer.
		tail := mrw.tailString()
		// Find last occurrence of "done":true
		lastDone := strings.LastIndex(tail, `"done":true`)
		if lastDone == -1 {
			lastDone = strings.LastIndex(tail, `"done": true`)
		}
		if lastDone > 0 {
			// Walk back to find the opening brace
			start := strings.LastIndex(tail[:lastDone], "{")
			if start >= 0 {
				fragment := tail[start:]
				// Find closing brace
				end := strings.Index(fragment, "}")
				if end >= 0 {
					var chunk ollamaFinalChunk
					if err := json.Unmarshal([]byte(fragment[:end+1]), &chunk); err == nil && chunk.Done {
						sample.InputTokens = chunk.PromptEvalCount
						sample.OutputTokens = chunk.EvalCount
						if chunk.EvalDuration > 0 && chunk.EvalCount > 0 {
							// Use Ollama's own measurement for more accurate TPS
							evalSec := float64(chunk.EvalDuration) / 1e9
							sample.DurationMs = evalSec * 1000
						}
					}
				}
			}
		}
	}

	if model != "" {
		a.Metrics.Record(sample)
	}
}

// genericProxy wraps the reverse proxy for non-metrics routes,
// performing capability checks.
func (a *Agent) genericProxy(w http.ResponseWriter, r *http.Request) {
	var model string
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		if err == nil {
			var peek struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal(bodyBytes, &peek)
			model = peek.Model
		}
	}

	if !a.checkCapabilities(w, r, model) {
		return
	}

	a.proxy.ServeHTTP(w, r)
}

func (a *Agent) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	status, err := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)
	if err != nil {
		sharedLog.Global.Errorf("Failed to get hardware status: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status.ID = a.ID
	status.Address = a.EffectiveAddress
	status.LastSeen = time.Now()

	// Use contexts for Ollama calls
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var ollamaErr error
	if active, err := a.Ollama.GetLoadedModels(ctx); err == nil {
		status.ActiveModels = active
	} else {
		sharedLog.Global.Warnf("Failed to fetch active models from Ollama: %v", err)
		ollamaErr = err
	}

	if local, err := a.Ollama.ListLocalModels(ctx); err == nil {
		status.LocalModels = local
	} else {
		sharedLog.Global.Warnf("Failed to list local models from Ollama: %v", err)
		if ollamaErr == nil {
			ollamaErr = err
		}
	}

	if ollamaErr != nil {
		http.Error(w, "Ollama unreachable: "+ollamaErr.Error(), http.StatusServiceUnavailable)
		return
	}

	// Attach the locally-measured capability map
	if caps := a.Metrics.Snapshot(); len(caps) > 0 {
		status.ModelCapabilities = caps
	}

	sharedLog.Global.Debugf("Telemetry status for agent %s: %+v", a.ID, status)
	json.NewEncoder(w).Encode(status)
}

func (a *Agent) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(a.Capabilities.GetPolicy())
		return
	}

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var p capabilities.Policy
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.Capabilities.UpdatePolicy(p)
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (a *Agent) HandleTasks(w http.ResponseWriter, r *http.Request) {
	sharedLog.Global.Debugf("Listing active tasks for agent %s", a.ID)
	json.NewEncoder(w).Encode(a.Tasks.ListTasks())
}

func (a *Agent) HandlePull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sharedLog.Global.Errorf("Invalid pull request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("pull-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "pull", req.Model)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		sharedLog.Global.Infof("Task %s: Pulling model %s", taskID, req.Model)
		err := a.Ollama.Pull(a.ctx, req.Model)
		a.Tasks.CompleteTask(taskID, err)
		if err != nil {
			sharedLog.Global.Errorf("Task %s: Pull failed for model %s: %v", taskID, req.Model, err)
		} else {
			sharedLog.Global.Infof("Task %s: Successfully pulled model %s", taskID, req.Model)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"task_id": taskID})
}

func (a *Agent) HandleUnload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sharedLog.Global.Errorf("Invalid unload request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sharedLog.Global.Infof("Unloading model %s", req.Model)
	if err := a.Ollama.Unload(ctx, req.Model); err != nil {
		sharedLog.Global.Errorf("Failed to unload model %s: %v", req.Model, err)
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
		sharedLog.Global.Errorf("Invalid delete request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sharedLog.Global.Infof("Deleting model %s from disk", req.Model)
	if err := a.Ollama.Delete(ctx, req.Model); err != nil {
		sharedLog.Global.Errorf("Failed to delete model %s: %v", req.Model, err)
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
		sharedLog.Global.Errorf("Invalid show request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sharedLog.Global.Debugf("Showing metadata for model %s", req.Model)
	result, err := a.Ollama.Show(ctx, req.Model)
	if err != nil {
		sharedLog.Global.Errorf("Failed to show model %s: %v", req.Model, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (a *Agent) HandleVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	version, err := a.Ollama.Version(ctx)
	if err != nil {
		sharedLog.Global.Errorf("Failed to get Ollama version: %v", err)
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
		sharedLog.Global.Errorf("Invalid create request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("create-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "create", req.Name)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		sharedLog.Global.Infof("Task %s: Creating model %s", taskID, req.Name)
		stream, _, err := a.Ollama.Create(a.ctx, req.Name, req.Modelfile)
		if err == nil {
			_, _ = io.Copy(io.Discard, stream)
			stream.Close()
			sharedLog.Global.Infof("Task %s: Successfully created model %s", taskID, req.Name)
		} else {
			sharedLog.Global.Errorf("Task %s: Failed to create model %s: %v", taskID, req.Name, err)
		}
		a.Tasks.CompleteTask(taskID, err)
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"task_id": taskID})
}

func (a *Agent) HandleCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sharedLog.Global.Errorf("Invalid copy request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	sharedLog.Global.Infof("Copying model %s to %s", req.Source, req.Destination)
	_, err := a.Ollama.Copy(ctx, req.Source, req.Destination)
	if err != nil {
		sharedLog.Global.Errorf("Failed to copy model %s to %s: %v", req.Source, req.Destination, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Agent) HandlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sharedLog.Global.Errorf("Invalid push request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("push-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "push", req.Name)

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		sharedLog.Global.Infof("Task %s: Pushing model %s", taskID, req.Name)
		stream, _, err := a.Ollama.Push(a.ctx, req.Name)
		if err == nil {
			_, _ = io.Copy(io.Discard, stream)
			stream.Close()
			sharedLog.Global.Infof("Task %s: Successfully pushed model %s", taskID, req.Name)
		} else {
			sharedLog.Global.Errorf("Task %s: Failed to push model %s: %v", taskID, req.Name, err)
		}
		a.Tasks.CompleteTask(taskID, err)
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"task_id": taskID})
}
func (a *Agent) StartLogShipper() {
	defer a.wg.Done()

	scheme := "http"
	if a.Config.TLS.Enabled {
		scheme = "https"
	}
	urlStr := a.BalancerURL
	if !strings.Contains(urlStr, "://") {
		urlStr = scheme + "://" + urlStr
	}
	urlStr = strings.TrimSuffix(urlStr, "/") + "/api/log/collect"

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if a.Logs == nil {
				continue
			}

			pending, err := a.Logs.FetchLogs(50)
			if err != nil || len(pending) == 0 {
				continue
			}

			var idsToDelete []int64
			for _, p := range pending {
				body, _ := json.Marshal(p.Entry)
				req, _ := http.NewRequestWithContext(a.ctx, "POST", urlStr, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				token := a.AgentKey
				if token == "" {
					token = a.Config.RemoteToken
				}
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}

				resp, err := a.httpClient.Do(req)
				if err == nil {
					if resp.StatusCode == http.StatusOK {
						idsToDelete = append(idsToDelete, p.ID)
					} else if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
						// Drop logs that are permanently rejected by the client
						sharedLog.Global.Warnf("Dropping log permanently rejected by balancer (status %d)", resp.StatusCode)
						idsToDelete = append(idsToDelete, p.ID)
					} else {
						resp.Body.Close()
						break
					}
					resp.Body.Close()
				} else {
					break
				}
			}
			_ = a.Logs.DeleteLogs(idsToDelete)

		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) Ship(entry models.LogEntry) {
	if a.Logs != nil {
		a.Logs.Ship(entry)
	}
}
