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

type metricEntry struct {
	nodeID, model string
	latency       time.Duration
	success       bool
}

// Balancer manages multiple agents and routes requests.
type Balancer struct {
	Address         string
	Agents          map[string]*models.NodeStatus
	Storage         *storage.SQLiteStorage
	Config          *config.Config
	PendingRequests map[string]int       // model_name -> count
	ModelLastUsed   map[string]time.Time // "node_id:model_name" -> last_time
	InProgressPulls map[string]time.Time // model_name -> start_time
	Queue           *RequestQueue
	NodeWorkloads   map[string]int                       // agent_addr -> count
	ClientAffinity  map[string]string                    // client_ip -> agent_id
	PerfCache       map[string]storage.PerformanceMetric // "node_id:model" -> metric
	perfMu          sync.RWMutex
	MetricCh        chan metricEntry
	Mu              sync.RWMutex
	stopCh          chan struct{}
	logChs          map[chan string]bool
	logMu           sync.Mutex
	httpClient      *http.Client
}

func NewBalancer(address string, dbPath string, cfg *config.Config) (*Balancer, error) {
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, err
	}

	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	b := &Balancer{
		Address:         address,
		Agents:          make(map[string]*models.NodeStatus),
		Storage:         s,
		Config:          cfg,
		PendingRequests: make(map[string]int),
		ModelLastUsed:   make(map[string]time.Time),
		InProgressPulls: make(map[string]time.Time),
		Queue:           NewRequestQueue(),
		NodeWorkloads:   make(map[string]int),
		ClientAffinity:  make(map[string]string),
		PerfCache:       make(map[string]storage.PerformanceMetric),
		MetricCh:        make(chan metricEntry, 1000),
		stopCh:          make(chan struct{}),
		logChs:          make(map[chan string]bool),
		httpClient:      &http.Client{Timeout: 5 * time.Second},
	}

	// Intercept log output
	log.SetOutput(b)

	return b, nil
}

func (b *Balancer) Write(p []byte) (n int, err error) {
	msg := string(p)
	os.Stderr.Write(p) // Also write to stderr

	b.logMu.Lock()
	for ch := range b.logChs {
		select {
		case ch <- msg:
		default:
		}
	}
	b.logMu.Unlock()
	return len(p), nil
}

func (b *Balancer) HandleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 100)
	b.logMu.Lock()
	b.logChs[ch] = true
	b.logMu.Unlock()

	defer func() {
		b.logMu.Lock()
		delete(b.logChs, ch)
		b.logMu.Unlock()
		close(ch)
	}()

	flusher, _ := w.(http.Flusher)
	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-b.stopCh:
			return
		}
	}
}

func (b *Balancer) Close() error {
	close(b.stopCh)
	return b.Storage.Close()
}

func (b *Balancer) StartBackgroundTasks() {
	b.StartPoller()
	b.StartKeepAliveCleaner()
	b.StartPerfCacheRefresher()
	b.StartMetricProcessor()
	b.StartWorkerPool(10) // 10 workers for routing
}

func (b *Balancer) StartMetricProcessor() {
	go func() {
		for {
			select {
			case m := <-b.MetricCh:
				b.Storage.RecordMetric(m.nodeID, m.model, m.latency, m.success)
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) StartPerfCacheRefresher() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.refreshPerfCache()
			case <-b.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (b *Balancer) refreshPerfCache() {
	b.Mu.RLock()
	// Get unique combinations of node IDs and model names from currently known state
	type entry struct{ nodeID, model string }
	entries := make([]entry, 0)
	for _, a := range b.Agents {
		for _, m := range a.ActiveModels {
			entries = append(entries, entry{a.ID, m})
		}
		for _, m := range a.LocalModels {
			entries = append(entries, entry{a.ID, m.Name})
		}
	}
	b.Mu.RUnlock()

	newCache := make(map[string]storage.PerformanceMetric)
	for _, e := range entries {
		perf, err := b.Storage.GetPerformance(e.nodeID, e.model)
		if err == nil {
			newCache[e.nodeID+":"+e.model] = perf
		}
	}

	b.perfMu.Lock()
	b.PerfCache = newCache
	b.perfMu.Unlock()
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

			id, addr, err := b.Route(req.Request, req.ClientIP)
			req.Response <- QueuedResponse{AgentID: id, AgentAddr: addr, Err: err}
		}
	}
}

func (b *Balancer) StartKeepAliveCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	pruneTicker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.cleanStaleModels()
			case <-pruneTicker.C:
				if err := b.Storage.PruneOldMetrics(2); err != nil {
					log.Printf("Failed to prune old metrics: %v", err)
				}
			case <-b.stopCh:
				ticker.Stop()
				pruneTicker.Stop()
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
			idx := strings.LastIndex(key, ":")
			if idx == -1 {
				continue
			}
			agentAddr := key[:idx]
			modelName := key[idx+1:]

			if agent, ok := b.Agents[agentAddr]; ok {
				toUnload = append(toUnload, struct{ nodeID, addr, model string }{agent.ID, agent.Address, modelName})
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
	return b.httpClient.Do(req)
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
			// Use internal httpClient with timeout
			resp, err := b.httpClient.Do(req)
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

				// Preserving some internal state
				status.State = currentAgent.State
				status.Errors = currentAgent.Errors
				status.Draining = currentAgent.Draining
				status.LastSeen = time.Now()

				b.Agents[a.Address] = &status

				// Update learned VRAM
				for _, m := range status.ActiveModels {
					if len(status.ActiveModels) == 1 {
						// Heuristic: if only one model is loaded, VRAMUsed is a good estimate
						b.Storage.UpdateModelVRAM(m, status.VRAMUsed)
					}
				}

				// Clear InProgressPulls if the model is now visible on any node (active or local)
				for m := range b.InProgressPulls {
					found := false
					for _, am := range status.ActiveModels {
						if am == m {
							found = true
							break
						}
					}
					if !found {
						for _, lm := range status.LocalModels {
							if lm.Name == m {
								found = true
								break
							}
						}
					}
					if found {
						log.Printf("Model %s discovered on node %s, clearing pull lock", m, a.ID)
						delete(b.InProgressPulls, m)
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

// Route finds the best agent for an inference request using adaptive heuristics and session stickiness.
func (b *Balancer) Route(req models.InferenceRequest, clientIP string) (string, string, error) {
	b.Mu.RLock()
	pending := b.PendingRequests[req.Model]
	affinityID := b.ClientAffinity[clientIP]

	var bestAgent *models.NodeStatus
	var bestScore = -1000.0

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
	now := time.Now()
	for _, a := range b.Agents {
		// Connectivity and state checks
		if time.Since(a.LastSeen) > 5*time.Second || a.Draining {
			continue
		}
		if a.State == models.StateBroken && now.Before(a.CooloffUntil) {
			continue
		}
		if a.VRAMTotal < minVRAM {
			continue
		}

		// Use Performance Cache
		b.perfMu.RLock()
		perf, ok := b.PerfCache[a.ID+":"+req.Model]
		b.perfMu.RUnlock()

		if !ok {
			// If no performance data exists, default to healthy assumption
			perf = storage.PerformanceMetric{SuccessRate: 1.0, AvgLatency: 1.0}
		}

		// 1. Foundation: CPU Load (Inverse)
		score := (1.0 - (a.CPUUsage / 100.0)) * b.Config.Weights.CPULoadWeight

		// 2. Least Connections: Penalize nodes with active workloads to prevent thundering herd
		workload := b.NodeWorkloads[a.Address]
		score -= float64(workload) * b.Config.Weights.WorkloadPenalty

		// 3. Thermal Protection
		if a.GPUTemperature > 80.0 {
			score *= 0.5
		}
		if a.GPUTemperature > 90.0 {
			continue // Critical thermal threshold
		}

		// 4. Historical Reliability (with Cold-Start defaults)
		successRate := perf.SuccessRate
		if successRate <= 0 {
			successRate = 1.0 // Assume healthy for new nodes
		}
		score *= (successRate * b.Config.Weights.SuccessRateWeight)

		if perf.AvgLatency > 0 {
			score *= ((1.0 / perf.AvgLatency) * b.Config.Weights.LatencyWeight)
		}

		// 5. Degradation Penalty
		if a.State == models.StateDegraded {
			score *= 0.5
		}

		// 6. Session Stickiness: Grant bonus for KV Cache locality
		if a.ID == affinityID {
			score += 2.0 // Stickiness bonus
		}

		// 7. Model Residency (Hot vs Warm vs Cold)
		isHot := false
		for _, m := range a.ActiveModels {
			if m == req.Model {
				isHot = true
				foundLoaded = true
				break
			}
		}

		if isHot {
			score += b.Config.Weights.LoadedModelBonus
		} else {
			// Check if model is on disk (Warm)
			isWarm := false
			for _, mInfo := range a.LocalModels {
				if mInfo.Name == req.Model {
					isWarm = true
					break
				}
			}

			if isWarm {
				score += b.Config.Weights.LocalModelBonus

				// VRAM Fragmentation Check: Penalize if Ollama must evict models
				freeVRAM := a.VRAMTotal - a.VRAMUsed
				if freeVRAM < minVRAM {
					score -= 1.0 // Eviction penalty
				}
			} else {
				// Cold start required (Pulling over network)
				score -= 5.0
			}
		}

		if score > bestScore {
			bestScore = score
			bestAgent = a
		}
	}
	b.Mu.RUnlock()

	if bestAgent != nil {
		b.Mu.Lock()
		b.ClientAffinity[clientIP] = bestAgent.ID
		b.Mu.Unlock()
	}

	// Auto-allocation logic
	if !foundLoaded || pending > b.Config.StaleThreshold {
		b.triggerAllocation(req.Model, minVRAM)
	}

	if bestAgent == nil {
		log.Printf("Routing failed: No suitable agent found for model %s (pending: %d)", req.Model, pending)
		return "", "", fmt.Errorf("no available agents with sufficient capabilities")
	}

	log.Printf("Routed model %s to agent %s (score: %.2f, pending: %d, affinity: %v)", req.Model, bestAgent.ID, bestScore, pending, bestAgent.ID == affinityID)
	return bestAgent.ID, bestAgent.Address, nil
}

func (b *Balancer) triggerAllocation(model string, minVRAM uint64) {
	b.Mu.Lock()
	if startTime, ok := b.InProgressPulls[model]; ok {
		// 10-minute safety timeout for pull lock
		if time.Since(startTime) < 10*time.Minute {
			b.Mu.Unlock()
			return
		}
	}
	b.InProgressPulls[model] = time.Now()
	b.Mu.Unlock()

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

func (b *Balancer) HandleAPIStatus(w http.ResponseWriter, _ *http.Request) {
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
	// Include models currently being pulled for the first time
	for m := range b.InProgressPulls {
		modelMap[m] = true
	}
	allModels := make([]string, 0, len(modelMap))
	for m := range modelMap {
		allModels = append(allModels, m)
	}
	sort.Strings(allModels)

	// Copy InProgressPulls
	pulls := make(map[string]time.Time)
	for m, t := range b.InProgressPulls {
		pulls[m] = t
	}

	// Copy NodeWorkloads
	workloads := make(map[string]int)
	for addr, count := range b.NodeWorkloads {
		workloads[addr] = count
	}

	status := models.ClusterStatus{
		Nodes:           b.Agents,
		PendingRequests: b.PendingRequests,
		InProgressPulls: pulls,
		NodeWorkloads:   workloads,
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
	if nodeID != "" || nodeAddr != "" {
		for _, a := range b.Agents {
			if (nodeAddr != "" && a.Address == nodeAddr) || (nodeID != "" && a.ID == nodeID) {
				targets = append(targets, a)
				if nodeAddr != "" {
					break
				}
			}
		}
	} else {
		for _, a := range b.Agents {
			targets = append(targets, a)
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
		go b.sendToAgent(agent.Address, "/models/unload", body)
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
	if nodeID != "" || nodeAddr != "" {
		for _, a := range b.Agents {
			if (nodeAddr != "" && a.Address == nodeAddr) || (nodeID != "" && a.ID == nodeID) {
				targets = append(targets, a)
				if nodeAddr != "" {
					break
				}
			}
		}
	} else {
		for _, a := range b.Agents {
			targets = append(targets, a)
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
		go b.sendToAgent(agent.Address, "/models/delete", body)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (b *Balancer) HandleTestInference(w http.ResponseWriter, r *http.Request) {
	model := r.FormValue("model")
	prompt := r.FormValue("prompt")
	nodeID := r.FormValue("node_id")
	nodeAddr := r.FormValue("node_addr")

	if model == "" || prompt == "" {
		// Try JSON body
		var req struct {
			Model    string `json:"model"`
			Prompt   string `json:"prompt"`
			NodeID   string `json:"node_id"`
			NodeAddr string `json:"node_addr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			model = req.Model
			prompt = req.Prompt
			nodeID = req.NodeID
			nodeAddr = req.NodeAddr
		}
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
	var resp *http.Response
	var agentID string
	var err error

	if nodeID != "" || nodeAddr != "" {
		b.Mu.RLock()
		var target *models.NodeStatus
		for _, a := range b.Agents {
			if (nodeAddr != "" && a.Address == nodeAddr) || (nodeID != "" && a.ID == nodeID) {
				target = a
				break
			}
		}
		b.Mu.RUnlock()

		if target == nil {
			http.Error(w, "Node not found", http.StatusNotFound)
			return
		}
		agentID = target.ID
		resp, err = b.sendToAgent(target.Address, "/inference", body)
	} else {
		resp, agentID, _, err = b.DoHedgedRequest(r.Context(), req.Model, "/inference", body, r.RemoteAddr)
	}

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

func (b *Balancer) HandleTags(w http.ResponseWriter, _ *http.Request) {
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
	resp, _, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/show", body, r.RemoteAddr)
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
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), req.Model, "/chat", body, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Concurrency Tracking: start
	if agentAddr != "" {
		b.Mu.Lock()
		b.NodeWorkloads[agentAddr]++
		b.Mu.Unlock()
		defer func() {
			b.Mu.Lock()
			b.NodeWorkloads[agentAddr]--
			b.Mu.Unlock()
		}()
	}

	b.finalizeProxy(w, resp, agentAddr, req.Model)
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
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), req.Model, "/inference", body, r.RemoteAddr)
	if err != nil {
		log.Printf("Failed to fulfill GenerateRequest for %s: %v", req.Model, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Concurrency Tracking: start
	if agentAddr != "" {
		b.Mu.Lock()
		b.NodeWorkloads[agentAddr]++
		b.Mu.Unlock()
		defer func() {
			b.Mu.Lock()
			b.NodeWorkloads[agentAddr]--
			b.Mu.Unlock()
		}()
	}

	b.finalizeProxy(w, resp, agentAddr, req.Model)
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
		select {
		case b.MetricCh <- metricEntry{agentAddr, modelName, latency, false}:
		default:
		}
		if errors.Is(err, ErrStalled) {
			log.Printf("Agent %s stalled during stream for model %s", agentAddr, modelName)
		} else {
			log.Printf("Stream error from %s: %v", agentAddr, err)
		}
		return
	}

	b.recordSuccess(agentAddr)
	select {
	case b.MetricCh <- metricEntry{agentAddr, modelName, latency, true}:
	default:
	}
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
			a.CooloffUntil = time.Now().Add(time.Duration(b.Config.CircuitBreaker.CooloffSec) * time.Second)
		} else {
			a.State = models.StateDegraded
		}
		if oldState != a.State {
			log.Printf("Node %s (%s) state changed: %s -> %s (errors: %d, cooloff until: %v)", a.ID, addr, oldState.String(), a.State.String(), a.Errors, a.CooloffUntil)
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
