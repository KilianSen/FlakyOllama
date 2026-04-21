package balancer

import (
	"FlakyOllama/pkg/balancer/jobs"
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"fmt"
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

	agentsCopy := make(map[string]*models.NodeStatus)
	for addr, agent := range snapshot.Agents {
		a := agent // local copy
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

	status := models.ClusterStatus{
		Nodes:           agentsCopy,
		PendingRequests: pendingRequestsCopy,
		InProgressPulls: snapshot.InProgressPulls,
		NodeWorkloads:   snapshot.NodeWorkloads,
		QueueDepth:      b.Queue.pq.Len(),
		ActiveWorkloads: totalWorkloads,
		AllModels:       allModels,
		TotalVRAM:       totalVRAM,
		UsedVRAM:        usedVRAM,
		TotalCPUCores:   totalCores,
		AvgCPUUsage:     avgCPU,
		AvgMemoryUsage:  avgMem,
		UptimeSeconds:   int64(time.Since(b.StartTime).Seconds()),
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

	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, "model_pull")
	b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
		j.Status = jobs.StatusRunning
		j.Message = "Starting pull for " + req.Model
	})

	go func() {
		body, _ := json.Marshal(map[string]string{"model": req.Model})
		var err error
		if req.NodeID != "" || req.NodeAddr != "" {
			snapshot := b.State.GetSnapshot()
			found := false
			for addr, agent := range snapshot.Agents {
				if agent.Address == req.NodeAddr || agent.ID == req.NodeID {
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
				s.InProgressPulls[req.Model] = time.Now()
			})
			b.Broadcast("/models/pull", body)
		}

		b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
			if err != nil {
				j.Status = jobs.StatusFailed
				j.Message = err.Error()
			} else {
				j.Status = jobs.StatusCompleted
				j.Message = "Pull triggered for " + req.Model
				j.Progress = 1.0
			}
		})
	}()

	b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"job_id": jobID,
		"status": "pull_triggered",
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

	jobID := generateJobID()
	b.Jobs.CreateJob(jobID, "model_delete")

	go func() {
		b.Jobs.UpdateJob(jobID, func(j *jobs.Job) { j.Status = jobs.StatusRunning })
		body, _ := json.Marshal(map[string]string{"model": name})
		b.Broadcast("/models/delete", body)
		b.Jobs.UpdateJob(jobID, func(j *jobs.Job) {
			j.Status = jobs.StatusCompleted
			j.Progress = 1.0
		})
	}()

	b.jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"job_id": jobID,
		"status": "delete_triggered",
	})
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

	var result models.InferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

	b.State.UpsertNode(addr, &models.NodeStatus{
		ID:       req.ID,
		Address:  addr,
		Tier:     req.Tier,
		State:    models.StateHealthy,
		Errors:   0,
		LastSeen: time.Now(),
	})

	logging.Global.Infof("Registered agent: %s at %s [Tier: %s]", req.ID, addr, req.Tier)
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
