package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

func (b *Balancer) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 0. Track load
	b.State.Do(func(s *ClusterState) {
		s.PendingRequests[req.Model]++
	})
	defer func() {
		b.State.Do(func(s *ClusterState) {
			s.PendingRequests[req.Model]--
		})
	}()

	// 1. Check Virtual Models
	var resolvedModel string
	var vConfig models.VirtualModelConfig
	found := false

	b.configMu.RLock()
	if cfg, ok := b.Config.VirtualModels[req.Model]; ok {
		vConfig = cfg
		found = true
	}
	b.configMu.RUnlock()

	if found && vConfig.Type == "pipeline" {
		// Not implemented in this context yet
		http.Error(w, "Pipeline models not supported in generate endpoint", http.StatusBadRequest)
		return
	}
	resolvedModel = req.Model

	req.Model = resolvedModel

	if b.Queue.QueueDepth() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated", http.StatusTooManyRequests)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	priority := b.getRequestPriority(r)
	if req.Priority == 0 {
		req.Priority = priority
	}

	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)
	contextHash := ""
	if req.Prompt != "" {
		contextHash = b.computeHash(req.Prompt)
	}

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(ctx, req.Model, "/inference", body, r.RemoteAddr, req.AllowHedging, req.Priority, contextHash)

	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentAddr, req.Model, r, surge)
}

func (b *Balancer) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	b.State.Do(func(s *ClusterState) {
		s.PendingRequests[req.Model]++
	})
	defer func() {
		b.State.Do(func(s *ClusterState) {
			s.PendingRequests[req.Model]--
		})
	}()

	if b.Queue.QueueDepth() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated", http.StatusTooManyRequests)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	priority := b.getRequestPriority(r)
	if req.Priority == 0 {
		req.Priority = priority
	}

	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)
	contextHash := ""
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		contextHash = b.computeHash(lastMsg.Content)
	}

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(ctx, req.Model, "/chat", body, r.RemoteAddr, req.AllowHedging, req.Priority, contextHash)

	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentAddr, req.Model, r, surge)
}

func (b *Balancer) HandleV1Register(w http.ResponseWriter, r *http.Request) {
	var status models.NodeStatus
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Token Verification
	if status.AgentKey == "" {
		http.Error(w, "Agent key required", http.StatusUnauthorized)
		return
	}

	ak, err := b.Storage.GetAgentKey(status.AgentKey)
	if err != nil || !ak.Active {
		logging.Global.Warnf("Registration attempt with invalid/inactive key: %s from %s", status.AgentKey, status.Address)
		http.Error(w, "Invalid agent key", http.StatusForbidden)
		return
	}

	// Update State
	status.ID = ak.NodeID
	status.LastSeen = time.Now()

	b.State.Do(func(s *ClusterState) {
		s.Agents[status.Address] = &status
	})

	logging.Global.Infof("Node %s registered from %s (%d models, GPU: %v)", status.ID, status.Address, len(status.LocalModels), status.HasGPU)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"node_id": status.ID, "status": "registered"})
}

func (b *Balancer) HandleV1Tags(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()
	uniqueModels := make(map[string]bool)
	for _, node := range snap.Agents {
		for _, m := range node.LocalModels {
			uniqueModels[m.Name] = true
		}
	}

	var tags models.TagsResponse
	for m := range uniqueModels {
		tags.Models = append(tags.Models, models.ModelInfo{
			Name:  m,
			Model: m,
		})
	}

	b.jsonResponse(w, http.StatusOK, tags)
}

func (b *Balancer) HandleOllamaEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(req)
	resp, _, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/api/embeddings", body, r.RemoteAddr, false, 10, "")
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

func (b *Balancer) getRequestPriority(r *http.Request) int {
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if user, ok := val.(models.User); ok && user.IsAdmin {
			return 100
		}
	}
	if val := r.Context().Value(auth.ContextKeyClientData); val != nil {
		if ck, ok := val.(models.ClientKey); ok && ck.Credits > 1000 {
			return 50
		}
	}
	return 10
}
