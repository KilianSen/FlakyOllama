package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

func (b *Balancer) HandleV1Catalog(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()
	uniqueModels := make(map[string]bool)

	// Physical models
	for _, n := range snap.Agents {
		if n.State != models.StateBroken && !n.Draining {
			for _, m := range n.LocalModels {
				uniqueModels[m.Name] = true
			}
		}
	}

	// Virtual models
	b.configMu.RLock()
	for m := range b.Config.VirtualModels {
		uniqueModels[m] = true
	}
	b.configMu.RUnlock()

	catalog := models.Catalog{
		GlobalRewardMultiplier: b.Config.GlobalRewardMultiplier,
		GlobalCostMultiplier:   b.Config.GlobalCostMultiplier,
		Models: make([]struct {
			Name         string  `json:"name"`
			RewardFactor float64 `json:"reward_factor"`
			CostFactor   float64 `json:"cost_factor"`
		}, 0),
	}

	for m := range uniqueModels {
		rf := 1.0
		if f, ok := b.Config.ModelRewardFactors[m]; ok {
			rf = f
		}
		cf := 1.0
		if f, ok := b.Config.ModelCostFactors[m]; ok {
			cf = f
		}
		catalog.Models = append(catalog.Models, struct {
			Name         string  `json:"name"`
			RewardFactor float64 `json:"reward_factor"`
			CostFactor   float64 `json:"cost_factor"`
		}{
			Name:         m,
			RewardFactor: rf,
			CostFactor:   cf,
		})
	}

	b.jsonResponse(w, http.StatusOK, catalog)
}

func (b *Balancer) HandleV1Me(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(auth.ContextKeyUser)
	if val == nil {
		b.jsonError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	user := val.(models.User)

	cks, err := b.Storage.GetClientKeysByUserID(user.ID)
	if err != nil {
		cks = []models.ClientKey{}
	}

	aks, err := b.Storage.GetAgentKeysByUserID(user.ID)
	if err != nil {
		aks = []models.AgentKey{}
	}

	b.jsonResponse(w, http.StatusOK, models.ProfileResponse{
		User:       user,
		ClientKeys: cks,
		AgentKeys:  aks,
	})
}

func (b *Balancer) HandleV1ClusterStatus(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()

	status := models.ClusterStatus{
		Nodes:                  snap.Agents,
		ActiveWorkloads:        snap.ActiveWorkloads,
		AvgCPUUsage:            snap.AvgCPUUsage,
		AvgMemUsage:            snap.AvgMemUsage,
		PendingRequests:        snap.PendingRequests,
		ModelRewardFactors:     b.Config.ModelRewardFactors,
		ModelCostFactors:       b.Config.ModelCostFactors,
		GlobalRewardMultiplier: b.Config.GlobalRewardMultiplier,
		GlobalCostMultiplier:   b.Config.GlobalCostMultiplier,
		OIDCEnabled:            b.Config.OIDC.Enabled,
		QueueDepth:             b.Queue.QueueDepth(),
		NodeWorkloads:          snap.NodeWorkloads,
		UptimeSeconds:          int64(time.Since(b.startTime).Seconds()),
	}

	// 1. Aggregate Physical Models
	uniqueModels := make(map[string]bool)
	for _, n := range snap.Agents {
		if n.State != models.StateBroken && !n.Draining {
			status.TotalVRAM += n.VRAMTotal
			status.UsedVRAM += n.VRAMUsed
			for _, m := range n.LocalModels {
				uniqueModels[m.Name] = true
			}
		}
	}

	// 2. Add Virtual Models
	b.configMu.RLock()
	for m := range b.Config.VirtualModels {
		uniqueModels[m] = true
	}
	b.configMu.RUnlock()

	status.AllModels = make([]string, 0, len(uniqueModels))
	for m := range uniqueModels {
		status.AllModels = append(status.AllModels, m)
	}

	// Mask Node IPs for non-admins
	isAdmin := false
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok && u.IsAdmin {
			isAdmin = true
		}
	}
	if !isAdmin {
		maskedNodes := make(map[string]*models.NodeStatus)
		for addr, n := range status.Nodes {
			nodeCopy := *n
			host, port, _ := net.SplitHostPort(addr)
			if host == "" {
				host = addr
			}
			maskedNodes["node-"+b.computeHash(host)[:6]+":"+port] = &nodeCopy
			nodeCopy.Address = "HIDDEN"
		}
		status.Nodes = maskedNodes
	}

	// Performance analytics (from cache)
	b.cacheMu.RLock()
	status.Performance = make(map[string]struct {
		AvgTTFT     float64 `json:"avg_ttft_ms"`
		AvgDuration float64 `json:"avg_duration_ms"`
		Requests    int     `json:"requests"`
	})
	for m, a := range b.perfCache {
		status.Performance[m] = struct {
			AvgTTFT     float64 `json:"avg_ttft_ms"`
			AvgDuration float64 `json:"avg_duration_ms"`
			Requests    int     `json:"requests"`
		}{
			AvgTTFT:     a.AvgTTFT,
			AvgDuration: a.AvgDuration,
			Requests:    a.Requests,
		}
	}
	b.cacheMu.RUnlock()

	// Totals
	stats, _ := b.Storage.GetTotalTokenStats()
	for _, s := range stats {
		status.TotalInputTokens += s.Input
		status.TotalOutputTokens += s.Output
		status.TotalReward += s.Reward
		status.TotalCost += s.Cost
	}

	b.jsonResponse(w, http.StatusOK, status)
}

func (b *Balancer) HandleV1Nodes(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()
	nodes := make([]*models.NodeStatus, 0, len(snap.Agents))

	isAdmin := false
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok && u.IsAdmin {
			isAdmin = true
		}
	}

	for _, n := range snap.Agents {
		if !isAdmin {
			nodeCopy := *n
			nodeCopy.Address = "HIDDEN"
			// Mask ID slightly to prevent direct mapping if ID is IP-based
			nodes = append(nodes, &nodeCopy)
		} else {
			nodes = append(nodes, n)
		}
	}
	b.jsonResponse(w, http.StatusOK, nodes)
}

func (b *Balancer) HandleV1Logs(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

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

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (b *Balancer) HandleV1LogHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	nodeID := r.URL.Query().Get("node_id")
	level := r.URL.Query().Get("level")
	query := r.URL.Query().Get("query")

	logs, err := b.Storage.SearchLogs(limit, nodeID, level, query)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, logs)
}

func (b *Balancer) HandleV1NodeDrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.State.Do(func(s *ClusterState) {
		for _, a := range s.Agents {
			if a.ID == id {
				a.Draining = true
				break
			}
		}
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "draining"})
}

func (b *Balancer) HandleV1NodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.State.Do(func(s *ClusterState) {
		for _, a := range s.Agents {
			if a.ID == id {
				a.Draining = false
				break
			}
		}
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "active"})
}

func (b *Balancer) HandleV1NodeDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.State.Do(func(s *ClusterState) {
		for addr, a := range s.Agents {
			if a.ID == id {
				delete(s.Agents, addr)
				break
			}
		}
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1ModelPull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	job := b.Jobs.CreateJob("pull")
	go func() {
		// Just record the request for now, actual pull is handled by tasks.go background loop
		b.Storage.CreateModelRequest(models.ModelRequest{
			ID:          job.ID,
			Type:        models.RequestPull,
			Model:       req.Model,
			NodeID:      req.NodeID,
			Status:      models.StatusPending,
			RequestedAt: time.Now(),
		})
	}()

	b.jsonResponse(w, http.StatusAccepted, map[string]string{"status": "accepted", "job_id": job.ID})
}

func (b *Balancer) HandleV1ModelDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	job := b.Jobs.CreateJob("delete")

	b.Storage.CreateModelRequest(models.ModelRequest{
		ID:          job.ID,
		Type:        models.RequestDelete,
		Model:       name,
		Status:      models.StatusPending,
		RequestedAt: time.Now(),
	})

	b.jsonResponse(w, http.StatusAccepted, map[string]string{"status": "accepted", "job_id": job.ID})
}

func (b *Balancer) HandleV1ModelUnload(w http.ResponseWriter, r *http.Request) {
	// Not implemented in agent yet
	b.jsonError(w, http.StatusNotImplemented, "not implemented")
}

func (b *Balancer) HandleV1ModelRequestsList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	reqs, err := b.Storage.ListModelRequests(models.ModelRequestStatus(status))
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, reqs)
}

func (b *Balancer) HandleV1ModelRequestApprove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.UpdateModelRequestStatus(id, models.StatusApproved); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (b *Balancer) HandleV1ModelRequestDecline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.UpdateModelRequestStatus(id, models.StatusRejected); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (b *Balancer) HandleV1ModelPolicySet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
		Banned bool   `json:"is_banned"`
		Pinned bool   `json:"is_pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := b.Storage.SetModelPolicy(req.Model, req.NodeID, req.Banned, req.Pinned); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "policy updated"})
}

func (b *Balancer) HandleV1KeySetStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type   string `json:"type"` // "client" or "agent"
		Key    string `json:"key"`
		Status string `json:"status"` // "active", "rejected"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	active := req.Status == "active"
	if err := b.Storage.SetKeyStatus(req.Type, req.Key, models.KeyStatus(req.Status), active); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "status updated"})
}

func (b *Balancer) HandleV1ClientKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := b.Storage.ListClientKeys()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, keys)
}

func (b *Balancer) HandleV1ClientKeyCreate(w http.ResponseWriter, r *http.Request) {
	var k models.ClientKey
	if err := json.NewDecoder(r.Body).Decode(&k); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// If it's an OIDC user creating a key
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		u := val.(models.User)
		k.UserID = u.ID
	}

	if k.Key == "" {
		k.Key = "ck_" + b.computeHash(fmt.Sprintf("%d_%s", time.Now().UnixNano(), k.Label))[:24]
	}
	k.Active = true
	k.Status = models.KeyStatusActive

	if err := b.Storage.CreateClientKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusCreated, k)
}

func (b *Balancer) HandleV1AgentKeysList(w http.ResponseWriter, r *http.Request) {
	keys, err := b.Storage.ListAgentKeys()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, keys)
}

func (b *Balancer) HandleV1AgentKeyCreate(w http.ResponseWriter, r *http.Request) {
	var k models.AgentKey
	if err := json.NewDecoder(r.Body).Decode(&k); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		u := val.(models.User)
		k.UserID = u.ID
	}

	if k.Key == "" {
		k.Key = "ak_" + b.computeHash(fmt.Sprintf("%d_%s", time.Now().UnixNano(), k.Label))[:24]
	}
	k.Active = true
	k.Status = models.KeyStatusActive
	k.Reputation = 1.0

	if err := b.Storage.CreateAgentKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusCreated, k)
}

func (b *Balancer) HandleV1UsersList(w http.ResponseWriter, r *http.Request) {
	users, err := b.Storage.ListUsers()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res := make([]models.UserWithKey, 0, len(users))
	for _, u := range users {
		cks, _ := b.Storage.GetClientKeysByUserID(u.ID)
		var key models.ClientKey
		if len(cks) > 0 {
			key = cks[0]
		}
		res = append(res, models.UserWithKey{User: u, Key: key})
	}
	b.jsonResponse(w, http.StatusOK, res)
}

func (b *Balancer) HandleV1UserUpdateQuota(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		QuotaLimit int64 `json:"quota_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	u, err := b.Storage.GetUserByID(id)
	if err != nil {
		b.jsonError(w, http.StatusNotFound, "user not found")
		return
	}

	u.QuotaLimit = req.QuotaLimit
	if err := b.Storage.UpdateUser(u); err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to update quota: "+err.Error())
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "quota updated"})
}

func (b *Balancer) HandleV1ClientKeyDelete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := b.Storage.DeleteClientKey(key); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1AgentKeyDelete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := b.Storage.DeleteAgentKey(key); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1UserDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.DeleteUser(id); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1QueueList(w http.ResponseWriter, r *http.Request) {
	b.jsonResponse(w, http.StatusOK, b.Queue.GetSnapshot())
}

func (b *Balancer) HandleV1QueueCancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if b.Queue.CancelRequest(id) {
		b.jsonResponse(w, http.StatusOK, map[string]string{"status": "cancelled"})
	} else {
		b.jsonError(w, http.StatusNotFound, "request not found in queue")
	}
}

func (b *Balancer) HandleV1JobStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, exists := b.Jobs.GetJob(id)
	if !exists {
		b.jsonError(w, http.StatusNotFound, "job not found")
		return
	}
	b.jsonResponse(w, http.StatusOK, job)
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

	b.configMu.Lock()
	*b.Config = newCfg
	b.configMu.Unlock()

	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if err := b.Config.SaveConfig(path); err != nil {
			logging.Global.Errorf("Failed to save config to %s: %v", path, err)
		}
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "config updated"})
}

func (b *Balancer) HandleV1LogCollect(w http.ResponseWriter, r *http.Request) {
	var entry models.LogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid log entry")
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Record to DB
	if err := b.Storage.RecordLog(entry.NodeID, string(entry.Level), entry.Component, entry.Message); err != nil {
		logging.Global.Debugf("Failed to record log to DB: %v", err)
	}

	// Broadcast to live loggers
	data, _ := json.Marshal(entry)
	b.broadcastLog(string(data))

	w.WriteHeader(http.StatusAccepted)
}

func (b *Balancer) jsonResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func (b *Balancer) jsonError(w http.ResponseWriter, code int, message string) {
	b.jsonResponse(w, code, map[string]string{"error": message})
}
