package balancer

import (
	"FlakyOllama/pkg/config"
	"FlakyOllama/pkg/models"
	"FlakyOllama/pkg/storage"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Balancer manages multiple agents and routes requests.
type Balancer struct {
	Address         string
	Agents          map[string]*models.NodeStatus
	Storage         *storage.SQLiteStorage
	Config          *config.Config
	PendingRequests map[string]int       // model_name -> count
	ModelLastUsed   map[string]time.Time // "node_id:model_name" -> last_time
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
	}, nil
}

func (b *Balancer) StartBackgroundTasks() {
	b.StartPoller()
	b.StartKeepAliveCleaner()
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
		http.Post("http://"+item.addr+"/models/unload", "application/json", bytes.NewBuffer(body))
	}
}

// Register registers a new agent.
func (b *Balancer) Register(req models.RegisterRequest) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	
	b.Agents[req.ID] = &models.NodeStatus{
		ID:      req.ID,
		Address: req.Address,
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
			resp, err := http.Get("http://" + a.Address + "/telemetry")
			if err != nil {
				log.Printf("Failed to poll agent %s: %v", a.ID, err)
				return
			}
			defer resp.Body.Close()

			var status models.NodeStatus
			if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
				b.Mu.Lock()
				b.Agents[a.ID] = &status
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

	// Get model requirements (mocked for now)
	minVRAM := uint64(0)
	if strings.Contains(req.Model, "7b") {
		minVRAM = 4 * 1024 * 1024 * 1024 // 4GB
	} else if strings.Contains(req.Model, "70b") {
		minVRAM = 40 * 1024 * 1024 * 1024 // 40GB
	}

	foundLoaded := false
	for _, a := range b.Agents {
		if time.Since(a.LastSeen) > time.Second {
			continue
		}
		if a.VRAMTotal < minVRAM {
			continue
		}

		perf, _ := b.Storage.GetPerformance(a.ID, req.Model)
		score := (1.0 - (a.CPUUsage / 100.0))
		if perf.SuccessRate > 0 {
			score *= perf.SuccessRate
		}
		if perf.AvgLatency > 0 {
			score *= (1.0 / perf.AvgLatency)
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
			score *= 2.0
		}

		if score > bestScore {
			bestScore = score
			bestAgent = a
		}
	}
	b.Mu.RUnlock()

	// Auto-allocation logic: trigger if model not loaded or queue is too deep
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
		if time.Since(a.LastSeen) > time.Second || a.VRAMTotal < minVRAM {
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
			go http.Post("http://"+a.Address+"/models/pull", "application/json", bytes.NewBuffer(body))
			return
		}
	}
}

// NewMux returns a mux with the balancer's handlers registered.
func (b *Balancer) NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", b.HandleRegister)
	mux.HandleFunc("/api/generate", b.HandleGenerate)
	return mux
}

// Serve starts the Balancer HTTP server.
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

	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		agentID, agentAddr, err := b.Route(req)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}

		start := time.Now()
		body, _ := json.Marshal(req)
		resp, err := http.Post("http://"+agentAddr+"/inference", "application/json", bytes.NewBuffer(body))
		
		success := err == nil && resp != nil && resp.StatusCode == http.StatusOK
		latency := time.Since(start)
		
		go b.Storage.RecordMetric(agentID, req.Model, latency, success)

		if !success {
			if resp != nil {
				resp.Body.Close()
			}
			lastErr = fmt.Errorf("agent %s failed (err: %v)", agentID, err)
			continue
		}
		defer resp.Body.Close()

		b.Mu.Lock()
		b.ModelLastUsed[agentID+":"+req.Model] = time.Now()
		b.Mu.Unlock()

		var result models.InferenceResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			lastErr = err
			continue
		}

		json.NewEncoder(w).Encode(result)
		return
	}

	http.Error(w, fmt.Sprintf("failed after %d retries: %v", maxRetries, lastErr), http.StatusServiceUnavailable)
}
