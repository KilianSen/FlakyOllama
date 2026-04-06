package balancer

import (
	"FlakyOllama/pkg/auth"
	"FlakyOllama/pkg/config"
	"FlakyOllama/pkg/metrics"
	"FlakyOllama/pkg/models"
	"FlakyOllama/pkg/storage"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	}
	log.Printf("Registered agent: %s at %s", req.ID, req.Address)
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
				// Preserving internal state while updating telemetry
				status.State = b.Agents[a.ID].State
				status.Errors = b.Agents[a.ID].Errors
				status.Draining = b.Agents[a.ID].Draining
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
				case models.StateHealthy: healthVal = 2.0
				case models.StateDegraded: healthVal = 1.0
				}
				metrics.NodeHealthStatus.WithLabelValues(a.ID).Set(healthVal)

				b.Mu.Unlock()
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
		return "", "", fmt.Errorf("no available agents with sufficient capabilities")
	}

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
	mux.HandleFunc("/api/tags", auth.Middleware(token, b.HandleTags))
	mux.HandleFunc("/status", b.HandleStatus)
	mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	mux.HandleFunc("/api/manage/node/drain", auth.Middleware(token, b.HandleNodeDrain))
	mux.HandleFunc("/api/manage/node/undrain", auth.Middleware(token, b.HandleNodeUndrain))
	
	return mux
}

func (b *Balancer) HandleNodeDrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		a.Draining = true
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Node not found", http.StatusNotFound)
	}
}

func (b *Balancer) HandleNodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		a.Draining = false
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Node not found", http.StatusNotFound)
	}
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

func (b *Balancer) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	b.Mu.Lock()
	b.PendingRequests[req.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[req.Model]--
		b.Mu.Unlock()
	}()

	resCh := b.Queue.Push(models.InferenceRequest{Model: req.Model, Stream: req.Stream}, 1)
	res := <-resCh
	if res.Err != nil {
		http.Error(w, res.Err.Error(), http.StatusServiceUnavailable)
		return
	}

	b.proxyStream(w, r.Context(), res.AgentID, res.AgentAddr, "/chat", req)
}

func (b *Balancer) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	b.Mu.Lock()
	b.PendingRequests[req.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[req.Model]--
		b.Mu.Unlock()
	}()

	resCh := b.Queue.Push(req, 1)
	res := <-resCh
	if res.Err != nil {
		http.Error(w, res.Err.Error(), http.StatusServiceUnavailable)
		return
	}

	b.proxyStream(w, r.Context(), res.AgentID, res.AgentAddr, "/inference", req)
}

func (b *Balancer) proxyStream(w http.ResponseWriter, ctx context.Context, agentID, agentAddr, path string, req interface{}) {
	modelName := ""
	if r, ok := req.(models.InferenceRequest); ok { modelName = r.Model }
	if r, ok := req.(models.ChatRequest); ok { modelName = r.Model }

	body, _ := json.Marshal(req)
	
	// Create request with context for cancellation propagation
	agentReq, _ := http.NewRequestWithContext(ctx, "POST", "http://"+agentAddr+path, bytes.NewBuffer(body))
	agentReq.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		agentReq.Header.Set("Authorization", "Bearer "+token)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(agentReq)
	
	status := "success"
	if err != nil || resp.StatusCode != http.StatusOK {
		status = "error"
		b.recordError(agentID)
		if err != nil {
			log.Printf("Proxy error to %s: %v", agentID, err)
		}
		http.Error(w, "agent failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	b.recordSuccess(agentID)
	latency := time.Since(start)
	
	// Update metrics
	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, status).Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(latency.Seconds())
	
	if modelName != "" {
		b.Mu.Lock()
		b.ModelLastUsed[agentID+":"+modelName] = time.Now()
		b.Mu.Unlock()
	}

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (b *Balancer) recordError(id string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		a.Errors++
		if a.Errors >= b.Config.CircuitBreaker.ErrorThreshold {
			a.State = models.StateBroken
		} else {
			a.State = models.StateDegraded
		}
	}
}

func (b *Balancer) recordSuccess(id string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[id]; ok {
		a.Errors = 0
		a.State = models.StateHealthy
	}
}
