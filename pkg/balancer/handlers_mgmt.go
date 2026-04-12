package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

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
		logging.Global.Infof("Unloading model %s from agent %s (%s)", model, agent.ID, agent.Address)
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
				logging.Global.Infof("Pulling model %s on agent %s (%s)", model, agent.ID, agent.Address)
				body, _ := json.Marshal(map[string]string{"model": model})
				go b.sendToAgent(agent.Address, "/models/pull", body)
				if nodeAddr != "" {
					break
				}
			}
		}
	} else {
		// Cluster-wide pull
		logging.Global.Infof("Pulling model %s cluster-wide", model)
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
		logging.Global.Infof("Deleting model %s from disk on agent %s (%s)", model, agent.ID, agent.Address)
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
		resp, agentID, _, err = b.DoHedgedRequest(r.Context(), req.Model, "/inference", body, r.RemoteAddr, false, 0)
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

	b.Mu.Lock()
	defer b.Mu.Unlock()

	b.Agents[req.Address] = &models.NodeStatus{
		ID:      req.ID,
		Address: req.Address,
		Tier:    req.Tier,
		State:   models.StateHealthy,
		Errors:  0,
	}
	logging.Global.Infof("Registered agent: %s at %s [Tier: %s] (resetting health)", req.ID, req.Address, req.Tier)

	w.WriteHeader(http.StatusOK)
}

func (b *Balancer) HandleLogCollect(w http.ResponseWriter, r *http.Request) {
	var entry models.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	select {
	case b.LogCh <- entry:
	default:
	}
	w.WriteHeader(http.StatusOK)
}
