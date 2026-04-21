package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (b *Balancer) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logging.Global.Infof("Incoming GenerateRequest for model %s (stream: %v)", req.Model, req.Stream)

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
		return
	}

	b.State.Do(func(s *state.ClusterState) {
		s.PendingRequests[req.Model]++
	})
	defer func() {
		b.State.Do(func(s *state.ClusterState) {
			s.PendingRequests[req.Model]--
		})
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	priority := b.getRequestPriority(r)
	if req.Priority > priority {
		// allow override if master admin or high credit, but cap at real priority
	} else if req.Priority == 0 {
		req.Priority = priority
	}

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(ctx, req.Model, "/inference", body, r.RemoteAddr, req.AllowHedging, req.Priority)
	if err != nil {
		logging.Global.Errorf("Failed to fulfill GenerateRequest for %s: %v", req.Model, err)
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentAddr, req.Model, r)
}

func (b *Balancer) HandleChat(w http.ResponseWriter, r *http.Request) {
	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logging.Global.Infof("Incoming ChatRequest for model %s (stream: %v)", req.Model, req.Stream)

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
		return
	}

	b.State.Do(func(s *state.ClusterState) {
		s.PendingRequests[req.Model]++
	})
	defer func() {
		b.State.Do(func(s *state.ClusterState) {
			s.PendingRequests[req.Model]--
		})
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	priority := b.getRequestPriority(r)
	if req.Priority > priority {
		// allow override
	} else if req.Priority == 0 {
		req.Priority = priority
	}

	body, _ := json.Marshal(req)
	resp, _, agentAddr, err := b.DoHedgedRequest(ctx, req.Model, "/chat", body, r.RemoteAddr, req.AllowHedging, req.Priority)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentAddr, req.Model, r)
}

func (b *Balancer) HandleShow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Clean model name
	if strings.HasPrefix(req.Model, "a.") {
		req.Model = strings.TrimPrefix(req.Model, "a.")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	body, _ := json.Marshal(req)
	resp, _, _, err := b.DoHedgedRequest(ctx, req.Model, "/show", body, r.RemoteAddr, false, 0)
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
	snapshot := b.State.GetSnapshot()

	modelMap := make(map[string]models.ModelInfo)
	for _, agent := range snapshot.Agents {
		// Include active models (running)
		for _, mName := range agent.ActiveModels {
			if _, ok := modelMap[mName]; !ok {
				modelMap[mName] = models.ModelInfo{
					Name:       mName,
					Model:      mName,
					ModifiedAt: time.Now(),
				}
			}
		}
		// Include local models (on disk)
		for _, mInfo := range agent.LocalModels {
			// Ensure Name is populated if it came from an older agent or is missing
			if mInfo.Name == "" {
				mInfo.Name = mInfo.Model
			}
			modelMap[mInfo.Model] = mInfo
		}
	}

	var modelList []models.ModelInfo
	for _, m := range modelMap {
		modelList = append(modelList, m)
	}

	sort.Slice(modelList, func(i, j int) bool {
		return modelList[i].Name < modelList[j].Name
	})

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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, _, _, err := b.DoHedgedRequest(ctx, req.Model, "/embeddings", body, r.RemoteAddr, false, 0)
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
	snapshot := b.State.GetSnapshot()

	type psModel struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	}
	var psModels []psModel
	seen := make(map[string]bool)

	for _, agent := range snapshot.Agents {
		for _, mName := range agent.ActiveModels {
			if !seen[mName] {
				psModels = append(psModels, psModel{Name: mName})
				seen[mName] = true
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"models": psModels})
}

func (b *Balancer) HandlePull(w http.ResponseWriter, r *http.Request) {
	// Standard Ollama /api/pull - proxy to HandleV1ModelPull (cluster-wide)
	b.HandleV1ModelPull(w, r)
}

func (b *Balancer) HandleDelete(w http.ResponseWriter, r *http.Request) {
	// Standard Ollama /api/delete - proxy to HandleV1ModelDelete (cluster-wide)
	b.HandleV1ModelDelete(w, r)
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
	logging.Global.Infof("Creating model %s cluster-wide", req.Name)
	b.Broadcast("/models/create", body)

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
	logging.Global.Infof("Copying model %s to %s cluster-wide", req.Source, req.Destination)
	b.Broadcast("/models/copy", body)

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
	logging.Global.Infof("Pushing model %s cluster-wide", req.Name)
	b.Broadcast("/models/push", body)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Push triggered for " + req.Name})
}
