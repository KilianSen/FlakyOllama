package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

func (b *Balancer) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Incoming GenerateRequest for model %s (stream: %v)", req.Model, req.Stream)

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
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

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), req.Model, "/inference", body, r.RemoteAddr, req.AllowHedging, req.Priority)
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

func (b *Balancer) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Incoming ChatRequest for model %s (stream: %v)", req.Model, req.Stream)

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
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

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), req.Model, "/chat", body, r.RemoteAddr, req.AllowHedging, req.Priority)
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

func (b *Balancer) HandleShow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	resp, _, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/show", body, r.RemoteAddr, false, 0)
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

func (b *Balancer) HandleEmbed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	resp, _, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/embeddings", body, r.RemoteAddr, false, 0)
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

func (b *Balancer) HandleVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": "0.1.0-flaky"})
}

func (b *Balancer) HandlePS(w http.ResponseWriter, _ *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	type psModel struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	}
	var models []psModel
	seen := make(map[string]bool)

	for _, agent := range b.Agents {
		for _, mName := range agent.ActiveModels {
			if !seen[mName] {
				models = append(models, psModel{Name: mName})
				seen[mName] = true
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"models": models})
}

func (b *Balancer) HandlePull(w http.ResponseWriter, r *http.Request) {
	// Standard Ollama /api/pull - proxy to HandleModelPull (cluster-wide)
	b.HandleModelPull(w, r)
}

func (b *Balancer) HandleDelete(w http.ResponseWriter, r *http.Request) {
	// Standard Ollama /api/delete - proxy to HandleModelDelete (cluster-wide)
	b.HandleModelDelete(w, r)
}

func (b *Balancer) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Modelfile string `json:"modelfile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	log.Printf("Creating model %s cluster-wide", req.Name)
	for _, agent := range b.Agents {
		if !agent.Draining && agent.State != models.StateBroken {
			go b.sendToAgent(agent.Address, "/models/create", body)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Creation triggered for " + req.Name})
}

func (b *Balancer) HandleCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	log.Printf("Copying model %s to %s cluster-wide", req.Source, req.Destination)
	for _, agent := range b.Agents {
		if !agent.Draining && agent.State != models.StateBroken {
			go b.sendToAgent(agent.Address, "/models/copy", body)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Copy triggered for " + req.Source})
}

func (b *Balancer) HandlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	log.Printf("Pushing model %s cluster-wide", req.Name)
	for _, agent := range b.Agents {
		if !agent.Draining && agent.State != models.StateBroken {
			go b.sendToAgent(agent.Address, "/models/push", body)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Push triggered for " + req.Name})
}
