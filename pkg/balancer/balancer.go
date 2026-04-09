package balancer

import (
	"FlakyOllama/pkg/auth"
	"FlakyOllama/pkg/config"
	"FlakyOllama/pkg/metrics"
	"FlakyOllama/pkg/models"
	"FlakyOllama/pkg/storage"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Balancer manages multiple agents and routes requests.
type Balancer struct {
	Address         string
	Agents          map[string]*models.NodeStatus
	Storage         *storage.SQLiteStorage
	Config          *config.Config
	PendingRequests map[string]int       // model_name -> count
	ModelLastUsed   map[string]time.Time // "node_id:model_name" -> last_time
	Queue           *RequestQueue
	Mu              sync.RWMutex
}

func NewBalancer(address string, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	return &Balancer{
		Address:         address,
		Agents:          make(map[string]*models.NodeStatus),
		Storage:         s,
		Config:          cfg,
		PendingRequests: make(map[string]int),
		ModelLastUsed:   make(map[string]time.Time),
		Queue:           NewRequestQueue(),
	}, nil
}

func (b *Balancer) StartBackgroundTasks() {
	b.StartPoller()
	b.StartKeepAliveCleaner()
	b.StartWorkerPool(10) // 10 workers for routing
}

func (b *Balancer) StartWorkerPool(workers int) {
	for i := 0; i < workers; i++ {
		go b.worker()
	}
}

func (b *Balancer) worker() {
	for {
		select {
		case <-b.Queue.Wait():
			req := b.Queue.Pop()
			if req == nil {
				continue
			}

			id, addr, err := b.Route(req.Request)
			req.Response <- QueuedResponse{AgentID: id, AgentAddr: addr, Err: err}
		}
	}
}

func (b *Balancer) StartKeepAliveCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			b.cleanStaleModels()
		}
	}()
}

func (b *Balancer) cleanStaleModels() {
	b.Mu.Lock()
	now := time.Now()
	keepAlive := time.Duration(b.Config.KeepAliveDurationSec) * time.Second

	toUnload := make([]struct{ nodeID, addr, model string }, 0)
	for key, lastTime := range b.ModelLastUsed {
		if now.Sub(lastTime) > keepAlive {
			parts := strings.Split(key, ":")
			if len(parts) != 2 {
				continue
			}
			nodeID, modelName := parts[0], parts[1]
			if agent, ok := b.Agents[nodeID]; ok {
				toUnload = append(toUnload, struct{ nodeID, addr, model string }{nodeID, agent.Address, modelName})
			}
			delete(b.ModelLastUsed, key)
		}
	}
	b.Mu.Unlock()

	for _, item := range toUnload {
		log.Printf("Unloading stale model %s from agent %s", item.model, item.nodeID)
		body, _ := json.Marshal(map[string]string{"model": item.model})
		b.sendToAgent(item.addr, "/models/unload", body)
	}
}

func (b *Balancer) sendToAgent(addr, path string, body []byte) (*http.Response, error) {
	req, _ := http.NewRequest("POST", "http://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}

// Register registers a new agent.
func (b *Balancer) Register(req models.RegisterRequest) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	b.Agents[req.ID] = &models.NodeStatus{
		ID:      req.ID,
		Address: req.Address,
		State:   models.StateHealthy,
		Errors:  0,
	}
	log.Printf("Registered agent: %s at %s (resetting health)", req.ID, req.Address)
}

// StartPoller polls registered agents at the configured frequency.
func (b *Balancer) StartPoller() {
	interval := time.Duration(b.Config.PollIntervalMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			b.pollAgents()
			metrics.QueueDepth.Set(float64(b.Queue.pq.Len()))
		}
	}()
}

func (b *Balancer) pollAgents() {
	b.Mu.RLock()
	agents := make([]*models.NodeStatus, 0, len(b.Agents))
	for _, a := range b.Agents {
		agents = append(agents, a)
	}
	b.Mu.RUnlock()

	for _, agent := range agents {
		go func(a *models.NodeStatus) {
			req, _ := http.NewRequest("GET", "http://"+a.Address+"/telemetry", nil)
			if token := os.Getenv("AGENT_TOKEN"); token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Failed to poll agent %s: %v", a.ID, err)
				b.recordError(a.ID)
				return
			}
			defer resp.Body.Close()

			var status models.NodeStatus
			if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
				b.Mu.Lock()
				currentAgent, ok := b.Agents[a.ID]
				if !ok {
					b.Mu.Unlock()
					return
				}

				// Preserving some internal state, but resetting errors on successful poll
				status.State = currentAgent.State
				status.Errors = currentAgent.Errors
				status.Draining = currentAgent.Draining

				// If we successfully polled but it was broken, consider it healthy again
				if status.State == models.StateBroken || status.State == models.StateDegraded {
					log.Printf("Agent %s recovered via successful poll", a.ID)
					status.State = models.StateHealthy
					status.Errors = 0
				}

				status.LastSeen = time.Now()

				b.Agents[a.ID] = &status

				// Update learned VRAM
				for _, m := range status.ActiveModels {
					if len(status.ActiveModels) == 1 {
						// Heuristic: if only one model is loaded, VRAMUsed is a good estimate
						b.Storage.UpdateModelVRAM(m, status.VRAMUsed)
					}
				}

				// Update metrics
				healthVal := 0.0
				switch status.State {
				case models.StateHealthy:
					healthVal = 2.0
				case models.StateDegraded:
					healthVal = 1.0
				}
				metrics.NodeHealthStatus.WithLabelValues(a.ID).Set(healthVal)

				b.Mu.Unlock()
			} else {
				log.Printf("Failed to decode telemetry for agent %s: %v", a.ID, err)
			}
		}(agent)
	}
}

// Route finds the best agent for an inference request.
func (b *Balancer) Route(req models.InferenceRequest) (string, string, error) {
	b.Mu.RLock()
	pending := b.PendingRequests[req.Model]

	var bestAgent *models.NodeStatus
	var bestScore float64 = -1.0

	// Get model requirements from learned metadata
	minVRAM, _ := b.Storage.GetModelVRAM(req.Model)
	if minVRAM == 0 {
		// Fallback for unknown models
		if strings.Contains(req.Model, "7b") {
			minVRAM = 4 * 1024 * 1024 * 1024
		} else if strings.Contains(req.Model, "70b") {
			minVRAM = 40 * 1024 * 1024 * 1024
		}
	}

	foundLoaded := false
	for _, a := range b.Agents {
		if time.Since(a.LastSeen) > time.Second || a.State == models.StateBroken || a.Draining {
			continue
		}
		if a.VRAMTotal < minVRAM {
			continue
		}

		perf, _ := b.Storage.GetPerformance(a.ID, req.Model)

		// Advanced Scoring Engine
		score := (1.0 - (a.CPUUsage / 100.0)) * b.Config.Weights.CPULoadWeight

		// Thermal Protection
		if a.GPUTemperature > 80.0 {
			score *= 0.5 // Heavy penalty above 80C
		}
		if a.GPUTemperature > 90.0 {
			continue // Skip node if above 90C
		}

		if perf.SuccessRate > 0 {
			score *= (perf.SuccessRate * b.Config.Weights.SuccessRateWeight)
		}
		if perf.AvgLatency > 0 {
			score *= ((1.0 / perf.AvgLatency) * b.Config.Weights.LatencyWeight)
		}

		// Degradation Penalty
		if a.State == models.StateDegraded {
			score *= 0.5
		}

		hasModel := false
		for _, m := range a.ActiveModels {
			if m == req.Model {
				hasModel = true
				foundLoaded = true
				break
			}
		}

		if hasModel {
			score *= b.Config.Weights.LoadedModelBonus
		}

		if score > bestScore {
			bestScore = score
			bestAgent = a
		}
	}
	b.Mu.RUnlock()

	// Auto-allocation logic
	if !foundLoaded || pending > b.Config.StaleThreshold {
		b.triggerAllocation(req.Model, minVRAM)
	}

	if bestAgent == nil {
		log.Printf("Routing failed: No suitable agent found for model %s (pending: %d)", req.Model, pending)
		return "", "", fmt.Errorf("no available agents with sufficient capabilities")
	}

	log.Printf("Routed model %s to agent %s (score: %.2f, pending: %d)", req.Model, bestAgent.ID, bestScore, pending)
	return bestAgent.ID, bestAgent.Address, nil
}

func (b *Balancer) triggerAllocation(model string, minVRAM uint64) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	for _, a := range b.Agents {
		if time.Since(a.LastSeen) > time.Second || a.VRAMTotal < minVRAM || a.State == models.StateBroken || a.Draining {
			continue
		}
		hasModel := false
		for _, m := range a.ActiveModels {
			if m == model {
				hasModel = true
				break
			}
		}
		if !hasModel {
			log.Printf("Triggering auto-allocation of model %s to agent %s", model, a.ID)
			body, _ := json.Marshal(map[string]string{"model": model})
			go b.sendToAgent(a.Address, "/models/pull", body)
			return
		}
	}
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *http.ServeMux {
	token := os.Getenv("BALANCER_TOKEN")
	mux := http.NewServeMux()

	mux.HandleFunc("/register", b.HandleRegister) // Register doesn't need auth usually or use a different secret
	mux.HandleFunc("/api/generate", auth.Middleware(token, b.HandleGenerate))
	mux.HandleFunc("/api/chat", auth.Middleware(token, b.HandleChat))
	mux.HandleFunc("/api/show", auth.Middleware(token, b.HandleShow))
	mux.HandleFunc("/api/tags", auth.Middleware(token, b.HandleTags))
	mux.HandleFunc("/status", b.HandleStatus)
	mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	mux.HandleFunc("/api/manage/node/drain", auth.Middleware(token, b.HandleNodeDrain))
	mux.HandleFunc("/api/manage/node/undrain", auth.Middleware(token, b.HandleNodeUndrain))
	mux.HandleFunc("/api/manage/model/unload", auth.Middleware(token, b.HandleModelUnload))
	mux.HandleFunc("/api/manage/model/pull", auth.Middleware(token, b.HandleModelPull))
	mux.HandleFunc("/api/manage/model/delete", auth.Middleware(token, b.HandleModelDelete))
	mux.HandleFunc("/api/manage/test", auth.Middleware(token, b.HandleTestInference))

	// OpenAI compatibility layer
	mux.HandleFunc("/v1/chat/completions", auth.Middleware(token, b.HandleOpenAIChat))
	mux.HandleFunc("/v1/completions", auth.Middleware(token, b.HandleOpenAICompletions))
	mux.HandleFunc("/v1/models", auth.Middleware(token, b.HandleOpenAIModels))

	return mux
}

func (b *Balancer) HandleNodeDrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	b.Mu.Lock()
	if a, ok := b.Agents[id]; ok {
		a.Draining = true
		b.Mu.Unlock()
		if r.Header.Get("HX-Request") == "true" {
			b.HandleStatus(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	} else {
		b.Mu.Unlock()
		http.Error(w, "Node not found", http.StatusNotFound)
	}
}

func (b *Balancer) HandleNodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	b.Mu.Lock()
	if a, ok := b.Agents[id]; ok {
		a.Draining = false
		b.Mu.Unlock()
		if r.Header.Get("HX-Request") == "true" {
			b.HandleStatus(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	} else {
		b.Mu.Unlock()
		http.Error(w, "Node not found", http.StatusNotFound)
	}
}

func (b *Balancer) HandleModelUnload(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	model := r.URL.Query().Get("model")

	b.Mu.RLock()
	agent, ok := b.Agents[nodeID]
	b.Mu.RUnlock()

	if !ok {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Printf("Unloading model %s from agent %s", model, nodeID)
	body, _ := json.Marshal(map[string]string{"model": model})
	_, err := b.sendToAgent(agent.Address, "/models/unload", body)
	if err != nil {
		http.Error(w, "Failed to unload model", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		b.HandleStatus(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (b *Balancer) HandleModelPull(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	model := r.FormValue("model")
	if model == "" {
		model = r.URL.Query().Get("model")
	}
	if model == "" {
		model = r.Header.Get("HX-Prompt")
	}

	if model == "" {
		http.Error(w, "Model name required", http.StatusBadRequest)
		return
	}

	b.Mu.RLock()
	defer b.Mu.RUnlock()

	if nodeID != "" {
		// Single node pull
		if agent, ok := b.Agents[nodeID]; ok {
			log.Printf("Pulling model %s on agent %s", model, nodeID)
			body, _ := json.Marshal(map[string]string{"model": model})
			go b.sendToAgent(agent.Address, "/models/pull", body)
		}
	} else {
		// Cluster-wide pull
		log.Printf("Pulling model %s cluster-wide", model)
		body, _ := json.Marshal(map[string]string{"model": model})
		for _, agent := range b.Agents {
			if !agent.Draining && agent.State != models.StateBroken {
				go b.sendToAgent(agent.Address, "/models/pull", body)
			}
		}
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<div class='bg-green-50 text-green-700 p-2 rounded text-xs animate-pulse'>Pull triggered for %s</div>", model)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (b *Balancer) HandleModelDelete(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	model := r.URL.Query().Get("model")

	b.Mu.RLock()
	agent, ok := b.Agents[nodeID]
	b.Mu.RUnlock()

	if !ok {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	log.Printf("Deleting model %s from disk on agent %s", model, nodeID)
	body, _ := json.Marshal(map[string]string{"model": model})
	_, err := b.sendToAgent(agent.Address, "/models/delete", body)
	if err != nil {
		http.Error(w, "Failed to delete model", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		b.HandleStatus(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (b *Balancer) HandleTestInference(w http.ResponseWriter, r *http.Request) {
	model := r.FormValue("model")
	prompt := r.FormValue("prompt")

	if model == "" || prompt == "" {
		fmt.Fprintf(w, "<div class='bg-red-50 text-red-700 p-4 rounded-lg'>Error: Model and Prompt are required.</div>")
		return
	}

	req := models.InferenceRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	b.Mu.Lock()
	b.PendingRequests[req.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[req.Model]--
		b.Mu.Unlock()
	}()

	body, _ := json.Marshal(req)
	resp, agentID, err := b.DoHedgedRequest(r.Context(), req.Model, "/inference", body)
	if err != nil {
		fmt.Fprintf(w, "<div class='bg-red-50 text-red-700 p-4 rounded-lg'>Error: %v</div>", err)
		return
	}
	defer resp.Body.Close()

	var result models.InferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(w, "<div class='bg-red-50 text-red-700 p-4 rounded-lg'>Error: Failed to decode response.</div>")
		return
	}

	fmt.Fprintf(w, `
		<div class='bg-indigo-50 border border-indigo-100 p-4 rounded-xl shadow-sm animate-in fade-in slide-in-from-bottom-2 duration-300'>
			<div class='flex justify-between items-center mb-2'>
				<span class='text-xs font-bold text-indigo-700 uppercase'>Response from %s</span>
				<span class='text-xs text-indigo-400'>%s</span>
			</div>
			<div class='text-gray-800 whitespace-pre-wrap leading-relaxed'>%s</div>
		</div>
	`, agentID, time.Now().Format("15:04:05"), result.Response)
}

func (b *Balancer) Serve() error {
	log.Printf("Balancer listening on %s", b.Address)
	return http.ListenAndServe(b.Address, b.NewMux())
}

func (b *Balancer) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Address fix for agents registering with 0.0.0.0 or empty address
	if strings.HasPrefix(req.Address, "0.0.0.0:") || strings.HasPrefix(req.Address, ":") {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		_, port, _ := net.SplitHostPort(req.Address)
		req.Address = net.JoinHostPort(host, port)
	}

	b.Register(req)
	w.WriteHeader(http.StatusOK)
}

func (b *Balancer) HandleTags(w http.ResponseWriter, r *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	modelMap := make(map[string]bool)
	for _, agent := range b.Agents {
		for _, model := range agent.ActiveModels {
			modelMap[model] = true
		}
	}

	var modelList []models.ModelInfo
	for m := range modelMap {
		modelList = append(modelList, models.ModelInfo{
			Name:       m,
			ModifiedAt: time.Now(),
		})
	}

	json.NewEncoder(w).Encode(models.TagsResponse{Models: modelList})
}

func (b *Balancer) HandleShow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	resp, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/show", body)
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

func (b *Balancer) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Incoming ChatRequest for model %s (stream: %v)", req.Model, req.Stream)

	b.Mu.Lock()
	b.PendingRequests[req.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[req.Model]--
		b.Mu.Unlock()
	}()

	body, _ := json.Marshal(req)
	resp, agentID, err := b.DoHedgedRequest(r.Context(), req.Model, "/chat", body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentID, req.Model)
}

func (b *Balancer) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Incoming GenerateRequest for model %s (stream: %v)", req.Model, req.Stream)

	b.Mu.Lock()
	b.PendingRequests[req.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[req.Model]--
		b.Mu.Unlock()
	}()

	body, _ := json.Marshal(req)
	resp, agentID, err := b.DoHedgedRequest(r.Context(), req.Model, "/inference", body)
	if err != nil {
		log.Printf("Failed to fulfill GenerateRequest for %s: %v", req.Model, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentID, req.Model)
}

func (b *Balancer) finalizeProxy(w http.ResponseWriter, resp *http.Response, agentID, modelName string) {
	start := time.Now()
	// Wrap with Stall Protection
	stallTimeout := time.Duration(b.Config.StallTimeoutSec) * time.Second
	reader := NewIdleTimeoutReader(resp.Body, stallTimeout)
	defer reader.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	_, err := io.Copy(w, reader)
	latency := time.Since(start)
	if err != nil {
		b.recordError(agentID)
		b.Storage.RecordMetric(agentID, modelName, latency, false)
		if errors.Is(err, ErrStalled) {
			log.Printf("Agent %s stalled during stream for model %s", agentID, modelName)
		} else {
			log.Printf("Stream error from %s: %v", agentID, err)
		}
		return
	}

	b.recordSuccess(agentID)
	b.Storage.RecordMetric(agentID, modelName, latency, true)
	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(latency.Seconds())

	b.Mu.Lock()
	b.ModelLastUsed[agentID+":"+modelName] = time.Now()
	b.Mu.Unlock()
}

func (b *Balancer) recordError(id string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		a.Errors++
		oldState := a.State
		if a.Errors >= b.Config.CircuitBreaker.ErrorThreshold {
			a.State = models.StateBroken
		} else {
			a.State = models.StateDegraded
		}
		if oldState != a.State {
			log.Printf("Node %s state changed: %s -> %s (errors: %d)", id, oldState.String(), a.State.String(), a.Errors)
		}
	}
}

func (b *Balancer) recordSuccess(id string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		if a.State != models.StateHealthy {
			log.Printf("Node %s recovered to Healthy state", id)
		}
		a.Errors = 0
		a.State = models.StateHealthy
	}
}
