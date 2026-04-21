package balancer

import (
	"FlakyOllama/pkg/balancer/jobs"
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// --- Helpers ---

func (b *Balancer) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func (b *Balancer) jsonError(w http.ResponseWriter, status int, message string) {
	b.jsonResponse(w, status, map[string]string{"error": message})
}

func generateJobID() string {
	return fmt.Sprintf("job_%d_%d", time.Now().Unix(), rand.Intn(1000))
}

// --- Handlers ---

func (b *Balancer) HandleV1Logs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Initial message to confirm connection
	initEntry, _ := json.Marshal(models.LogEntry{
		Timestamp: time.Now(),
		NodeID:    "balancer",
		Level:     models.LevelInfo,
		Component: "balancer",
		Message:   "Cloud sync established. Streaming live telemetry...",
	})
	fmt.Fprintf(w, "data: %s\n\n", string(initEntry))

	// Historic logs
	if recent, err := b.Storage.GetRecentLogs(100); err == nil {
		// Reverse to show in chronological order
		for i := len(recent) - 1; i >= 0; i-- {
			l := recent[i]
			entry, _ := json.Marshal(models.LogEntry{
				Timestamp: l.Timestamp,
				NodeID:    l.NodeID,
				Level:     models.LogLevel(l.Level),
				Component: l.Component,
				Message:   l.Message,
			})
			fmt.Fprintf(w, "data: %s\n\n", string(entry))
		}
	}
	flusher.Flush()

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

func (b *Balancer) HandleV1ClusterStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()

	totalWorkloads := 0
	pendingRequestsCopy := make(map[string]int)
	for m, c := range snapshot.PendingRequests {
		pendingRequestsCopy[m] = c
		totalWorkloads += c
	}

	modelMap := make(map[string]bool)
	var totalVRAM, usedVRAM uint64
	var totalCores int
	var sumCPU, sumMem float64
	var healthyNodes int

	tokenStats, _ := b.Storage.GetTotalTokenStats()
	agentKeys, _ := b.Storage.ListAgentKeys()
	repMap := make(map[string]float64)
	for _, k := range agentKeys {
		repMap[k.Key] = k.Reputation
	}

	var totalInput, totalOutput int64
	var totalReward, totalCost float64

	agentsCopy := make(map[string]*models.NodeStatus)
	for addr, agent := range snapshot.Agents {
		a := *agent // full copy
		if stats, ok := tokenStats[agent.ID]; ok {
			a.InputTokens = int(stats.Input)
			a.OutputTokens = int(stats.Output)
			a.TokenReward = stats.Reward
			totalInput += stats.Input
			totalOutput += stats.Output
			totalReward += stats.Reward
			totalCost += stats.Cost
		}
		if r, ok := repMap[agent.AgentKey]; ok {
			a.Reputation = r
		} else {
			a.Reputation = 1.0 // Default
		}
		agentsCopy[addr] = &a

		if agent.State == models.StateHealthy {
			totalVRAM += agent.VRAMTotal
			usedVRAM += agent.VRAMUsed
			totalCores += agent.CPUCores
			sumCPU += agent.CPUUsage
			sumMem += agent.MemoryUsage
			healthyNodes++
		}

		for _, model := range agent.LocalModels {
			modelMap[model.Model] = true
		}
		for _, model := range agent.ActiveModels {
			modelMap[model] = true
		}
	}

	var avgCPU, avgMem float64
	if healthyNodes > 0 {
		avgCPU = sumCPU / float64(healthyNodes)
		avgMem = sumMem / float64(healthyNodes)
	}

	for m := range snapshot.InProgressPulls {
		modelMap[m] = true
	}

	allModels := make([]string, 0, len(modelMap))
	for m := range modelMap {
		allModels = append(allModels, m)
	}
	sort.Strings(allModels)

	// Performance Analytics
	perf, _ := b.Storage.GetPerformanceAnalytics()

	status := models.ClusterStatus{
		Nodes:             agentsCopy,
		PendingRequests:   pendingRequestsCopy,
		InProgressPulls:   snapshot.InProgressPulls,
		NodeWorkloads:     snapshot.NodeWorkloads,
		QueueDepth:        b.Queue.pq.Len(),
		ActiveWorkloads:   totalWorkloads,
		AllModels:         allModels,
		TotalVRAM:         totalVRAM,
		UsedVRAM:          usedVRAM,
		TotalCPUCores:     totalCores,
		AvgCPUUsage:       avgCPU,
		AvgMemoryUsage:    avgMem,
		UptimeSeconds:     int64(time.Since(b.StartTime).Seconds()),
		ModelPolicies:     snapshot.ModelPolicies,
		TotalInputTokens:  int(totalInput),
		TotalOutputTokens: int(totalOutput),
		TotalReward:       totalReward,
		TotalCost:         totalCost,
		Performance: make(map[string]struct {
			AvgTTFT     float64 `json:"avg_ttft_ms"`
			AvgDuration float64 `json:"avg_duration_ms"`
			Requests    int     `json:"requests"`
		}),
	}

	for m, p := range perf {
		status.Performance[m] = struct {
			AvgTTFT     float64 `json:"avg_ttft_ms"`
			AvgDuration float64 `json:"avg_duration_ms"`
			Requests    int     `json:"requests"`
		}{AvgTTFT: p.AvgTTFT, AvgDuration: p.AvgDuration, Requests: p.Requests}
	}

	b.jsonResponse(w, http.StatusOK, status)
}

func (b *Balancer) HandleV1Nodes(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()
	nodes := make([]models.NodeStatus, 0, len(snapshot.Agents))
	for _, agent := range snapshot.Agents {
		nodes = append(nodes, agent)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Address < nodes[j].Address
	})

	b.jsonResponse(w, http.StatusOK, nodes)
}

func (b *Balancer) HandleV1NodeDrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		b.jsonError(w, http.StatusBadRequest, "node id required")
		return
	}

	found := false
	b.State.Do(func(s *state.ClusterState) {
		for _, a := range s.Agents {
			if a.ID == id || a.Address == id {
				a.Draining = true
				found = true
			}
		}
	})

	if found {
		b.jsonResponse(w, http.StatusOK, map[string]string{"status": "draining", "id": id})
	} else {
		b.jsonError(w, http.StatusNotFound, "node not found")
	}
}

func (b *Balancer) HandleV1NodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		b.jsonError(w, http.StatusBadRequest, "node id required")
		return
	}

	found := false
	b.State.Do(func(s *state.ClusterState) {
		for _, a := range s.Agents {
			if a.ID == id || a.Address == id {
				a.Draining = false
				found = true
			}
		}
	})

	if found {
		b.jsonResponse(w, http.StatusOK, map[string]string{"status": "active", "id": id})
	} else {
		b.jsonError(w, http.StatusNotFound, "node not found")
	}
}

func (b *Balancer) HandleV1JobStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, ok := b.Jobs.GetJob(id)
	if !ok {
		b.jsonError(w, http.StatusNotFound, "job not found")
		return
	}
	b.jsonResponse(w, http.StatusOK, job)
}

func (b *Balancer) HandleV1ModelUnload(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if strings.HasPrefix(name, "a.") {
		name = strings.TrimPrefix(name, "a.")
	}
	var req struct {
		NodeID   string `json:"node_id"`
		NodeAddr string `json:"node_addr"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if name == "" {
		b.jsonError(w, http.StatusBadRequest, "model name required")
		return
	}

	snapshot := b.State.GetSnapshot()
	var targets []string
	if req.NodeID != "" || req.NodeAddr != "" {
		for addr, a := range snapshot.Agents {
			if a.Address == req.NodeAddr || a.ID == req.NodeID {
				targets = append(targets, addr)
			}
		}
	} else {
		for addr := range snapshot.Agents {
			targets = append(targets, addr)
		}
	}

	if len(targets) == 0 {
		b.jsonError(w, http.StatusNotFound, "no target nodes found")
		return
	}

	body, _ := json.Marshal(map[string]string{"model": name})
	for _, addr := range targets {
		logging.Global.Infof("Unloading model %s from agent %s", name, addr)
		go b.sendToAgentWithContext(context.Background(), addr, "/models/unload", body)
	}

	b.jsonResponse(w, http.StatusAccepted, map[string]string{"status": "unload_triggered", "model": name})
}

func (b *Balancer) HandleV1ModelPull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model    string `json:"model"`
		NodeID   string `json:"node_id"`
		NodeAddr string `json:"node_addr"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Model = r.FormValue("model")
	}

	if strings.HasPrefix(req.Model, "a.") {
		req.Model = strings.TrimPrefix(req.Model, "a.")
	}

	if req.Model == "" {
		b.jsonError(w, http.StatusBadRequest, "model name required")
		return
	}

	// Manual Approval Mode
	if b.Config.EnableModelApproval {
		requestID := generateJobID()
		err := b.Storage.CreateModelRequest(models.ModelRequest{
			ID:          requestID,
			Type:        models.RequestPull,
			Model:       req.Model,
			NodeID:      req.NodeID,
			Status:      models.StatusPending,
			RequestedAt: time.Now(),
		})
		if err != nil {
			b.jsonError(w, http.StatusInternalServerError, "failed to create approval request: "+err.Error())
			return
		}
		b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
			"request_id": requestID,
			"status":     "approval_pending",
			"message":    "Request submitted for manual approval",
		})
		return
	}

	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, "model_pull")
	b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
		j.Status = jobs.StatusRunning
		j.Message = "Starting pull for " + req.Model
	})

	go func() {
		b.executePull(jobID, req.Model, req.NodeID, req.NodeAddr)
	}()

	b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"job_id": jobID,
		"status": "pull_triggered",
	})
}

func (b *Balancer) executePull(jobID, model, nodeID, nodeAddr string) {
	body, _ := json.Marshal(map[string]string{"model": model})
	var err error
	if nodeID != "" || nodeAddr != "" {
		snapshot := b.State.GetSnapshot()
		found := false
		for addr, agent := range snapshot.Agents {
			if agent.Address == nodeAddr || agent.ID == nodeID {
				_, err = b.sendToAgentWithContext(context.Background(), addr, "/models/pull", body)
				found = true
				break
			}
		}
		if !found {
			err = fmt.Errorf("node not found")
		}
	} else {
		// Cluster-wide pull
		b.State.Do(func(s *state.ClusterState) {
			s.InProgressPulls[model] = time.Now()
		})
		b.Broadcast("/models/pull", body)
	}

	b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
		if err != nil {
			j.Status = jobs.StatusFailed
			j.Message = err.Error()
		} else {
			j.Status = jobs.StatusCompleted
			j.Message = "Pull triggered for " + model
			j.Progress = 1.0
		}
	})
}
func (b *Balancer) HandleV1ModelDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if strings.HasPrefix(name, "a.") {
		name = strings.TrimPrefix(name, "a.")
	}
	if name == "" {
		b.jsonError(w, http.StatusBadRequest, "model name required")
		return
	}

	// Manual Approval Mode
	if b.Config.EnableModelApproval {
		requestID := generateJobID()
		err := b.Storage.CreateModelRequest(models.ModelRequest{
			ID:          requestID,
			Type:        models.RequestDelete,
			Model:       name,
			Status:      models.StatusPending,
			RequestedAt: time.Now(),
		})
		if err != nil {
			b.jsonError(w, http.StatusInternalServerError, "failed to create approval request: "+err.Error())
			return
		}
		b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
			"request_id": requestID,
			"status":     "approval_pending",
			"message":    "Delete request submitted for manual approval",
		})
		return
	}

	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, "model_delete")

	go func() {
		b.executeDelete(jobID, name)
	}()

	b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"job_id": jobID,
		"status": "delete_triggered",
	})
}

func (b *Balancer) executeDelete(jobID, name string) {
	b.Jobs.UpdateJob(jobID, func(j *jobs.Job) { j.Status = jobs.StatusRunning })
	body, _ := json.Marshal(map[string]string{"model": name})
	b.Broadcast("/models/delete", body)
	b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
		j.Status = jobs.StatusCompleted
		j.Message = "Delete triggered for " + name
		j.Progress = 1.0
	})
}

func (b *Balancer) HandleV1ModelRequestsList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	reqs, err := b.Storage.ListModelRequests(status)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, reqs)
}

func (b *Balancer) HandleV1ModelRequestApprove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	req, err := b.Storage.GetModelRequest(id)
	if err != nil {
		b.jsonError(w, http.StatusNotFound, "request not found")
		return
	}

	if req.Status != models.StatusPending {
		b.jsonError(w, http.StatusBadRequest, "request is not pending")
		return
	}

	if err := b.Storage.UpdateModelRequestStatus(id, models.StatusApproved); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger the actual action
	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, string(req.Type))

	go func() {
		switch req.Type {
		case models.RequestPull:
			b.executePull(jobID, req.Model, req.NodeID, "")
		case models.RequestDelete:
			b.executeDelete(jobID, req.Model)
		}
	}()

	b.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "approved",
		"job_id": jobID,
	})
}

func (b *Balancer) HandleV1ModelRequestDecline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.UpdateModelRequestStatus(id, models.StatusDeclined); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "declined"})
}

func (b *Balancer) HandleV1ModelPolicySet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
		Banned bool   `json:"banned"`
		Pinned bool   `json:"pinned"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := b.Storage.SetModelPolicy(req.Model, req.NodeID, req.Banned, req.Pinned); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update in-memory state
	b.State.DoAsync(func(s *state.ClusterState) {
		if _, ok := s.ModelPolicies[req.Model]; !ok {
			s.ModelPolicies[req.Model] = make(map[string]struct{ Banned, Pinned bool })
		}
		s.ModelPolicies[req.Model][req.NodeID] = struct{ Banned, Pinned bool }{Banned: req.Banned, Pinned: req.Pinned}
	})

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "policy_updated"})
}

// Client Key Handlers
func (b *Balancer) HandleV1ClientKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := b.Storage.ListClientKeys()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, keys)
}

func (b *Balancer) HandleV1ClientKeyCreate(w http.ResponseWriter, r *http.Request) {
	var req models.ClientKey
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		req.Key = generateJobID()
	}
	if err := b.Storage.CreateClientKey(req); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusCreated, req)
}

// Agent Key Handlers
func (b *Balancer) HandleV1AgentKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := b.Storage.ListAgentKeys()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, keys)
}

func (b *Balancer) HandleV1AgentKeyCreate(w http.ResponseWriter, r *http.Request) {
	var req models.AgentKey
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		req.Key = generateJobID()
	}
	if err := b.Storage.CreateAgentKey(req); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusCreated, req)
}

// Public / Self-service Handlers
func (b *Balancer) HandleV1Catalog(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()

	type modelInfo struct {
		Name   string  `json:"name"`
		Reward float64 `json:"reward_factor"`
		Cost   float64 `json:"cost_factor"`
	}

	var catalog []modelInfo
	for _, m := range snapshot.AllModels {
		reward := 1.0
		if f, ok := b.Config.ModelRewardFactors[m]; ok {
			reward = f
		}
		cost := 1.0
		if f, ok := b.Config.ModelCostFactors[m]; ok {
			cost = f
		}

		catalog = append(catalog, modelInfo{
			Name: m, Reward: reward, Cost: cost,
		})
	}

	b.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"global_reward_multiplier": b.Config.GlobalRewardMultiplier,
		"global_cost_multiplier":   b.Config.GlobalCostMultiplier,
		"models":                   catalog,
	})
}

func (b *Balancer) HandleV1Me(w http.ResponseWriter, r *http.Request) {
	token, _ := r.Context().Value(auth.ContextKeyToken).(string)
	if token == "" {
		b.jsonError(w, http.StatusUnauthorized, "no identity found")
		return
	}

	// Try Client Key
	if ck, err := b.Storage.GetClientKey(token); err == nil {
		b.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"type":  "client",
			"label": ck.Label,
			"data":  ck,
		})
		return
	}

	// Try Agent Key
	if ak, err := b.Storage.GetAgentKey(token); err == nil {
		b.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"type":  "agent",
			"label": ak.Label,
			"data":  ak,
		})
		return
	}

	b.jsonError(w, http.StatusNotFound, "identity not found in registry")
}

func (b *Balancer) HandleV1TestInference(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Model == "" || req.Prompt == "" {
		b.jsonError(w, http.StatusBadRequest, "model and prompt are required")
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

	body, _ := json.Marshal(req)
	resp, agentID, _, err := b.DoHedgedRequest(ctx, req.Model, "/inference", body, r.RemoteAddr, false, 0)

	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	// Capture usage
	clientKey, _ := r.Context().Value(auth.ContextKeyToken).(string)
	bodyBytes, _ := io.ReadAll(resp.Body)
	go b.captureUsage(agentID, req.Model, bodyBytes, clientKey)

	var result models.InferenceResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to decode response")
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"agent_id": agentID,
		"response": result.Response,
	})
}

func (b *Balancer) HandleV1Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.ID == "" || req.Address == "" {
		b.jsonError(w, http.StatusBadRequest, "id and address are required")
		return
	}

	// Address fix for agents registering with 0.0.0.0 or empty address
	addr := req.Address
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if host == "0.0.0.0" || host == "" {
			remoteHost, _, _ := net.SplitHostPort(r.RemoteAddr)
			addr = net.JoinHostPort(remoteHost, port)
		}
	}

	// Get authenticated token from context
	token, _ := r.Context().Value(auth.ContextKeyToken).(string)

	b.State.UpsertNode(addr, &models.NodeStatus{
		ID:       req.ID,
		AgentKey: token,
		Address:  addr,
		Tier:     req.Tier,
		HasGPU:   req.HasGPU,
		GPUModel: req.GPUModel,
		State:    models.StateHealthy,
		Errors:   0,
		LastSeen: time.Now(),
	})

	logging.Global.Infof("Registered agent: %s at %s [Tier: %s, GPU: %v (%s)]", req.ID, addr, req.Tier, req.HasGPU, req.GPUModel)
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (b *Balancer) HandleV1LogCollect(w http.ResponseWriter, r *http.Request) {
	var entry models.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	select {
	case b.LogCh <- entry:
	default:
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *Balancer) HandleV1ConfigGet(w http.ResponseWriter, r *http.Request) {
	b.jsonResponse(w, http.StatusOK, b.Config)
}

func (b *Balancer) HandleV1ConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid config: "+err.Error())
		return
	}

	// Update the live config
	*b.Config = newCfg

	// Optionally save to disk if CONFIG_PATH is set
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if err := b.Config.SaveConfig(path); err != nil {
			logging.Global.Errorf("Failed to save config to %s: %v", path, err)
		}
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "config updated"})
}
