package agent

import (
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

// Agent handles local telemetry and proxies requests to Ollama.
type Agent struct {
	ID               string
	AgentKey         string
	Address          string
	EffectiveAddress string
	BalancerURL      string
	Monitor          *monitoring.Monitor
	Ollama           *ollama.Client
	Config           *config.Config
	Tasks            *tasks.TaskManager
	Logs             *agentLogging.DiskQueue

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	httpClient *http.Client
	httpServer *http.Server
	proxy      *httputil.ReverseProxy
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

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize DiskQueue for logs
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("agent_logs_%s.db", id))
	dq, err := agentLogging.NewDiskQueue(dbPath)
	if err != nil {
		sharedLog.Global.Errorf("Failed to initialize disk queue: %v", err)
	}

	// Initialize Reverse Proxy
	target, _ := url.Parse(ollamaURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
			},
		},
		Timeout: 30 * time.Second,
	}

	a := &Agent{
		ID:               id,
		AgentKey:         key,
		Address:          address,
		EffectiveAddress: address,
		BalancerURL:      balancerURL,
		Monitor:          monitoring.NewMonitor(),
		Ollama:           ollama.NewClient(ollamaURL),
		Config:           cfg,
		Tasks:            tasks.NewTaskManager(),
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
	agentReq, _ := http.NewRequestWithContext(a.ctx, "POST", a.BalancerURL+"/register", bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")

	token := a.AgentKey
	if token == "" {
		token = a.Config.RemoteToken
	}
	if token != "" {
		agentReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := a.httpClient.Do(agentReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to register with balancer: status %d", resp.StatusCode)
	}

	return nil
}

func (a *Agent) NewMux() *http.ServeMux {
	token := a.Config.AuthToken
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/telemetry", auth.Middleware([]string{token}, nil, a.HandleTelemetry))
	mux.HandleFunc("/tasks", auth.Middleware([]string{token}, nil, a.HandleTasks))

	// Proxy routes using ReverseProxy
	mux.HandleFunc("/v1/", auth.Middleware([]string{token}, nil, a.proxy.ServeHTTP))
	mux.HandleFunc("/inference", auth.Middleware([]string{token}, nil, a.proxy.ServeHTTP))
	mux.HandleFunc("/chat", auth.Middleware([]string{token}, nil, a.proxy.ServeHTTP))
	mux.HandleFunc("/embeddings", auth.Middleware([]string{token}, nil, a.proxy.ServeHTTP))

	// Direct handlers for more control
	mux.HandleFunc("/show", auth.Middleware([]string{token}, nil, a.HandleShow))
	mux.HandleFunc("/version", auth.Middleware([]string{token}, nil, a.HandleVersion))

	// Async Task handlers
	mux.HandleFunc("/models/pull", auth.Middleware([]string{token}, nil, a.HandlePull))
	mux.HandleFunc("/models/unload", auth.Middleware([]string{token}, nil, a.HandleUnload))
	mux.HandleFunc("/models/delete", auth.Middleware([]string{token}, nil, a.HandleDelete))
	mux.HandleFunc("/models/create", auth.Middleware([]string{token}, nil, a.HandleCreate))
	mux.HandleFunc("/models/copy", auth.Middleware([]string{token}, nil, a.HandleCopy))
	mux.HandleFunc("/models/push", auth.Middleware([]string{token}, nil, a.HandlePush))

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

	a.wg.Add(2)
	go a.StartLogShipper()
	go a.StartRegistrationLoop()

	if a.Config.TLS.Enabled {
		return a.httpServer.ListenAndServeTLS(a.Config.TLS.CertFile, a.Config.TLS.KeyFile)
	}
	return a.httpServer.ListenAndServe()
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

func (a *Agent) HandleTelemetry(w http.ResponseWriter, r *http.Request) {
	status, err := a.Monitor.GetStatus(a.Config.MaxVRAMAllocated, a.Config.MaxCPUAllocated)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status.ID = a.ID
	status.Address = a.EffectiveAddress
	status.LastSeen = time.Now()

	// Use contexts for Ollama calls
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if active, err := a.Ollama.GetLoadedModels(ctx); err == nil {
		status.ActiveModels = active
	}
	if local, err := a.Ollama.ListLocalModels(ctx); err == nil {
		status.LocalModels = local
	}

	json.NewEncoder(w).Encode(status)
}

func (a *Agent) HandleTasks(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(a.Tasks.ListTasks())
}

func (a *Agent) HandlePull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("pull-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "pull", req.Model)

	go func() {
		sharedLog.Global.Infof("Task %s: Pulling model %s", taskID, req.Model)
		err := a.Ollama.Pull(a.ctx, req.Model)
		a.Tasks.CompleteTask(taskID, err)
		if err != nil {
			sharedLog.Global.Errorf("Task %s: Pull failed: %v", taskID, err)
		} else {
			sharedLog.Global.Infof("Task %s: Pull completed", taskID)
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := a.Ollama.Unload(ctx, req.Model); err != nil {
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := a.Ollama.Delete(ctx, req.Model); err != nil {
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := a.Ollama.Show(ctx, req.Model)
	if err != nil {
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

	taskID := fmt.Sprintf("create-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "create", req.Name)

	go func() {
		stream, _, err := a.Ollama.Create(a.ctx, req.Name, req.Modelfile)
		if err == nil {
			_, _ = io.Copy(io.Discard, stream)
			stream.Close()
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	_, err := a.Ollama.Copy(ctx, req.Source, req.Destination)
	if err != nil {
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("push-%d", time.Now().UnixNano())
	a.Tasks.AddTask(taskID, "push", req.Name)

	go func() {
		stream, _, err := a.Ollama.Push(a.ctx, req.Name)
		if err == nil {
			_, _ = io.Copy(io.Discard, stream)
			stream.Close()
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
				if err == nil && resp.StatusCode == http.StatusOK {
					idsToDelete = append(idsToDelete, p.ID)
					resp.Body.Close()
				} else {
					if resp != nil {
						resp.Body.Close()
					}
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
