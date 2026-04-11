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
	"sort"
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
	InProgressPulls map[string]bool      // model_name -> is_pulling
	Queue           *RequestQueue
	Mu              sync.RWMutex
	stopCh          chan struct{}
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
		InProgressPulls: make(map[string]bool),
		Queue:           NewRequestQueue(),
		stopCh:          make(chan struct{}),
	}, nil
}

func (b *Balancer) Close() error {
	close(b.stopCh)
	return b.Storage.Close()
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
		case <-b.stopCh:
			return
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
		for {
			select {
			case <-ticker.C:
				b.cleanStaleModels()
			case <-b.stopCh:
				ticker.Stop()
				return
			}
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

	b.Agents[req.Address] = &models.NodeStatus{
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
		for {
			select {
			case <-ticker.C:
				b.pollAgents()
				metrics.QueueDepth.Set(float64(b.Queue.pq.Len()))
			case <-b.stopCh:
				ticker.Stop()
				return
			}
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
				log.Printf("Failed to poll agent %s (%s): %v", a.ID, a.Address, err)
				b.recordError(a.Address)
				return
			}
			defer resp.Body.Close()

			var status models.NodeStatus
			if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
				b.Mu.Lock()
				currentAgent, ok := b.Agents[a.Address]
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
					log.Printf("Agent %s (%s) recovered via successful poll", a.ID, a.Address)
					status.State = models.StateHealthy
					status.Errors = 0
				}

				status.LastSeen = time.Now()

				b.Agents[a.Address] = &status

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
				metrics.NodeHealthStatus.WithLabelValues(a.ID, a.Address).Set(healthVal)

				b.Mu.Unlock()
			} else {
				log.Printf("Failed to decode telemetry for agent %s (%s): %v", a.ID, a.Address, err)
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
	b.Mu.Lock()
	if b.InProgressPulls[model] {
		b.Mu.Unlock()
		return
	}
	b.InProgressPulls[model] = true
	b.Mu.Unlock()

	defer func() {
		// We'll reset this after some time or after we see the model loaded
		// For now, let's just reset it after 10 seconds to allow retry if it failed
		time.AfterFunc(10*time.Second, func() {
			b.Mu.Lock()
			delete(b.InProgressPulls, model)
			b.Mu.Unlock()
		})
	}()

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

func (b *Balancer) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Origin, Accept")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (b *Balancer) HandleAPIStatus(w http.ResponseWriter, r *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	totalWorkloads := 0
	for _, count := range b.PendingRequests {
		totalWorkloads += count
	}

	modelMap := make(map[string]bool)
	for _, agent := range b.Agents {
		for _, model := range agent.LocalModels {
			modelMap[model.Name] = true
		}
		for _, model := range agent.ActiveModels {
			modelMap[model] = true
		}
	}
	allModels := make([]string, 0, len(modelMap))
	for m := range modelMap {
		allModels = append(allModels, m)
	}
	sort.Strings(allModels)

	status := models.ClusterStatus{
		Nodes:           b.Agents,
		PendingRequests: b.PendingRequests,
		QueueDepth:      b.Queue.pq.Len(),
		ActiveWorkloads: totalWorkloads,
		AllModels:       allModels,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *http.ServeMux {
	token := os.Getenv("BALANCER_TOKEN")
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/register", b.HandleRegister) // Register doesn't need auth usually or use a different secret
	mux.HandleFunc("/api/generate", auth.Middleware(token, b.HandleGenerate))
	mux.HandleFunc("/api/chat", auth.Middleware(token, b.HandleChat))
	mux.HandleFunc("/api/show", auth.Middleware(token, b.HandleShow))
	mux.HandleFunc("/api/tags", auth.Middleware(token, b.HandleTags))
	mux.HandleFunc("/api/status", auth.Middleware(token, b.HandleAPIStatus))
	mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	mux.HandleFunc("/api/manage/node/drain", auth.Middleware(token, b.HandleNodeDrain))
	mux.HandleFunc("/api/manage/node/undrain", auth.Middleware(token, b.HandleNodeUndrain))
	mux.HandleFunc("/api/manage/model/unload", auth.Middleware(token, b.HandleModelUnload))
	mux.HandleFunc("/api/manage/model/pull", auth.Middleware(token, b.HandleModelPull))
	mux.HandleFunc("/api/manage/model/delete", auth.Middleware(token, b.HandleModelDelete))
	mux.HandleFunc("/api/manage/test", auth.Middleware(token, b.HandleTestInference))
	mux.HandleFunc("/api/logs", b.HandleLogs)

	// OpenAI compatibility layer
	mux.HandleFunc("/v1/chat/completions", auth.Middleware(token, b.HandleOpenAIChat))
	mux.HandleFunc("/v1/completions", auth.Middleware(token, b.HandleOpenAICompletions))
	mux.HandleFunc("/v1/models", auth.Middleware(token, b.HandleOpenAIModels))

	return mux
}

func (b *Balancer) HandleNodeDrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	addr := r.URL.Query().Get("addr")
	b.Mu.Lock()
	found := false
	for _, a := range b.Agents {
		if (addr != "" && a.Address == addr) || (id != "" && a.ID == id) {
			a.Draining = true
			found = true
			if addr != "" {
				break
			}
		}
	}
	b.Mu.Unlock()

	if found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		http.Error(w, "Node not found", http.StatusNotFound)
	}
}

func (b *Balancer) HandleNodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	addr := r.URL.Query().Get("addr")
	b.Mu.Lock()
	found := false
	for _, a := range b.Agents {
		if (addr != "" && a.Address == addr) || (id != "" && a.ID == id) {
			a.Draining = false
			found = true
			if addr != "" {
				break
			}
		}
	}
	b.Mu.Unlock()

	if found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		http.Error(w, "Node not found", http.StatusNotFound)
	}
}

func (b *Balancer) HandleModelUnload(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	nodeAddr := r.URL.Query().Get("addr")
	model := r.URL.Query().Get("model")

	b.Mu.RLock()
	var targets []*models.NodeStatus
	for _, a := range b.Agents {
		if (nodeAddr != "" && a.Address == nodeAddr) || (nodeID != "" && a.ID == nodeID) {
			targets = append(targets, a)
			if nodeAddr != "" {
				break
			}
		}
	}
	b.Mu.RUnlock()

	if len(targets) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	body, _ := json.Marshal(map[string]string{"model": model})
	for _, agent := range targets {
		log.Printf("Unloading model %s from agent %s (%s)", model, agent.ID, agent.Address)
		b.sendToAgent(agent.Address, "/models/unload", body)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (b *Balancer) HandleModelPull(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	nodeAddr := r.URL.Query().Get("addr")
	model := r.FormValue("model")
	if model == "" {
		model = r.URL.Query().Get("model")
	}
	if model == "" {
		// Try JSON body
		var req struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		model = req.Model
	}

	if model == "" {
		http.Error(w, "Model name required", http.StatusBadRequest)
		return
	}

	b.Mu.RLock()
	defer b.Mu.RUnlock()

	if nodeID != "" || nodeAddr != "" {
		// Single node or group pull
		for _, agent := range b.Agents {
			if (nodeAddr != "" && agent.Address == nodeAddr) || (nodeID != "" && agent.ID == nodeID) {
				log.Printf("Pulling model %s on agent %s (%s)", model, agent.ID, agent.Address)
				body, _ := json.Marshal(map[string]string{"model": model})
				go b.sendToAgent(agent.Address, "/models/pull", body)
				if nodeAddr != "" {
					break
				}
			}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Pull triggered for " + model})
}

func (b *Balancer) HandleModelDelete(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("id")
	nodeAddr := r.URL.Query().Get("addr")
	model := r.URL.Query().Get("model")

	b.Mu.RLock()
	var targets []*models.NodeStatus
	for _, a := range b.Agents {
		if (nodeAddr != "" && a.Address == nodeAddr) || (nodeID != "" && a.ID == nodeID) {
			targets = append(targets, a)
			if nodeAddr != "" {
				break
			}
		}
	}
	b.Mu.RUnlock()

	if len(targets) == 0 {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	body, _ := json.Marshal(map[string]string{"model": model})
	for _, agent := range targets {
		log.Printf("Deleting model %s from disk on agent %s (%s)", model, agent.ID, agent.Address)
		b.sendToAgent(agent.Address, "/models/delete", body)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (b *Balancer) HandleTestInference(w http.ResponseWriter, r *http.Request) {
	model := r.FormValue("model")
	prompt := r.FormValue("prompt")

	if model == "" || prompt == "" {
		// Try JSON body
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		model = req.Model
		prompt = req.Prompt
	}

	if model == "" || prompt == "" {
		http.Error(w, "Model and Prompt are required", http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	var result models.InferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		http.Error(w, "Failed to decode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"agent_id": agentID,
		"response": result.Response,
	})
}

func (b *Balancer) Serve() error {
	log.Printf("Balancer listening on %s", b.Address)
	return http.ListenAndServe(b.Address, b.CORS(b.NewMux()))
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

	modelMap := make(map[string]models.ModelInfo)
	for _, agent := range b.Agents {
		// Include active models (running)
		for _, mName := range agent.ActiveModels {
			if _, ok := modelMap[mName]; !ok {
				modelMap[mName] = models.ModelInfo{
					Name:       mName,
					ModifiedAt: time.Now(),
				}
			}
		}
		// Include local models (on disk)
		for _, mInfo := range agent.LocalModels {
			modelMap[mInfo.Name] = mInfo
		}
	}

	var modelList []models.ModelInfo
	for _, m := range modelMap {
		modelList = append(modelList, m)
	}

	w.Header().Set("Content-Type", "application/json")
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

func (b *Balancer) finalizeProxy(w http.ResponseWriter, resp *http.Response, agentAddr, modelName string) {
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
		b.recordError(agentAddr)
		b.Storage.RecordMetric(agentAddr, modelName, latency, false)
		if errors.Is(err, ErrStalled) {
			log.Printf("Agent %s stalled during stream for model %s", agentAddr, modelName)
		} else {
			log.Printf("Stream error from %s: %v", agentAddr, err)
		}
		return
	}

	b.recordSuccess(agentAddr)
	b.Storage.RecordMetric(agentAddr, modelName, latency, true)
	// We still use ID for some metrics if we want to aggregate by "name",
	// but here we should probably use Address or ID:Address
	agentID := agentAddr
	b.Mu.RLock()
	if a, ok := b.Agents[agentAddr]; ok {
		agentID = a.ID
	}
	b.Mu.RUnlock()

	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(latency.Seconds())

	b.Mu.Lock()
	b.ModelLastUsed[agentAddr+":"+modelName] = time.Now()
	b.Mu.Unlock()
}

func (b *Balancer) recordError(addr string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[addr]; ok {
		a.Errors++
		oldState := a.State
		if a.Errors >= b.Config.CircuitBreaker.ErrorThreshold {
			a.State = models.StateBroken
		} else {
			a.State = models.StateDegraded
		}
		if oldState != a.State {
			log.Printf("Node %s (%s) state changed: %s -> %s (errors: %d)", a.ID, addr, oldState.String(), a.State.String(), a.Errors)
		}
	}
}

func (b *Balancer) recordSuccess(addr string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[addr]; ok {
		if a.State != models.StateHealthy {
			log.Printf("Node %s (%s) recovered to Healthy state", a.ID, addr)
		}
		a.Errors = 0
		a.State = models.StateHealthy
	}
}
