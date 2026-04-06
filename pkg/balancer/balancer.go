package balancer

import (
	"FlakyOllama/pkg/models"
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
	Address string
	Agents  map[string]*models.NodeStatus
	Mu      sync.RWMutex
}

func NewBalancer(address string) *Balancer {
	return &Balancer{
		Address: address,
		Agents:  make(map[string]*models.NodeStatus),
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

// StartPoller polls registered agents at 10Hz.
func (b *Balancer) StartPoller() {
	ticker := time.NewTicker(100 * time.Millisecond)
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
func (b *Balancer) Route(req models.InferenceRequest) (string, error) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	var bestAgent *models.NodeStatus
	var minLoad float64 = 101.0 // CPU usage is 0-100

	// Get model requirements (mocked for now)
	minVRAM := uint64(0)
	if strings.Contains(req.Model, "7b") {
		minVRAM = 4 * 1024 * 1024 * 1024 // 4GB
	} else if strings.Contains(req.Model, "70b") {
		minVRAM = 40 * 1024 * 1024 * 1024 // 40GB
	}

	// Very simple load balancing for now: find agent with lowest CPU usage
	// and that has the model loaded (or can load it).
	for _, a := range b.Agents {
		// Basic health check (last seen < 1s ago)
		if time.Since(a.LastSeen) > time.Second {
			continue
		}

		// Capability check: VRAM
		if a.VRAMTotal < minVRAM {
			continue
		}

		// Prefer agents that already have the model loaded
		hasModel := false
		for _, m := range a.ActiveModels {
			if m == req.Model {
				hasModel = true
				break
			}
		}

		if hasModel {
			if a.CPUUsage < minLoad {
				minLoad = a.CPUUsage
				bestAgent = a
			}
		}
	}

	// If no agent has the model loaded, pick the one with lowest load overall
	if bestAgent == nil {
		for _, a := range b.Agents {
			if time.Since(a.LastSeen) > time.Second {
				continue
			}
			// Capability check: VRAM
			if a.VRAMTotal < minVRAM {
				continue
			}
			if a.CPUUsage < minLoad {
				minLoad = a.CPUUsage
				bestAgent = a
			}
		}
	}

	if bestAgent == nil {
		return "", fmt.Errorf("no available agents with sufficient capabilities")
	}

	return bestAgent.Address, nil
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

	// Retry logic
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		agentAddr, err := b.Route(req)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}

		body, _ := json.Marshal(req)
		resp, err := http.Post("http://"+agentAddr+"/inference", "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Agent %s failed, retrying... (%v)", agentAddr, err)
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("agent returned status %d", resp.StatusCode)
			continue
		}

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
