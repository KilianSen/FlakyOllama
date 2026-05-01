package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/config"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

func (b *Balancer) HandlePublicInfo(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()
	healthyNodes := 0
	uniqueModels := make(map[string]bool)
	for _, n := range snap.Agents {
		if n.State != models.StateBroken && !n.Draining {
			healthyNodes++
			for _, m := range n.LocalModels {
				uniqueModels[m.Name] = true
			}
		}
	}
	b.configMu.RLock()
	for m := range b.Config.VirtualModels {
		uniqueModels[m] = true
	}
	b.configMu.RUnlock()

	b.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"oidc_enabled":     b.Config.OIDC.Enabled,
		"healthy_nodes":    healthyNodes,
		"model_count":      len(uniqueModels),
		"active_workloads": snap.ActiveWorkloads,
	})
}

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

	usage, _ := b.Storage.GetUserQuotaUsage(user.ID)

	b.jsonResponse(w, http.StatusOK, models.ProfileResponse{
		User:       user,
		ClientKeys: cks,
		AgentKeys:  aks,
		QuotaUsage: usage,
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
		VirtualModels:          b.Config.VirtualModels,
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

	// Totals + per-node stats
	tokenStats, _ := b.Storage.GetTotalTokenStats()
	tpsStats, _ := b.Storage.GetRecentThroughput(60)
	for _, s := range tokenStats {
		status.TotalInputTokens += s.Input
		status.TotalOutputTokens += s.Output
		status.TotalReward += s.Reward
		status.TotalCost += s.Cost
	}

	// Populate per-node stats by matching AgentKey to token_usage node_id.
	// Work on a fresh copy map so we never mutate shared ClusterState pointers.
	enriched := make(map[string]*models.NodeStatus, len(status.Nodes))
	for addr, n := range status.Nodes {
		nc := *n
		if s, ok := tokenStats[nc.AgentKey]; ok {
			nc.InputTokens = s.Input
			nc.OutputTokens = s.Output
			nc.TokenReward = s.Reward
		}
		if tps, ok := tpsStats[nc.AgentKey]; ok {
			nc.TokensPerSecond = tps
		}
		enriched[addr] = &nc
	}
	status.Nodes = enriched

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
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	var addr string
	b.State.View(func(s ClusterState) {
		for a, n := range s.Agents {
			if n.ID == req.NodeID {
				addr = a
				break
			}
		}
	})

	if addr == "" {
		b.jsonError(w, http.StatusNotFound, "node not found")
		return
	}

	body, _ := json.Marshal(map[string]string{"model": req.Model})
	resp, err := b.sendToAgentWithContext(r.Context(), addr, "/api/models/unload", body)
	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.jsonError(w, resp.StatusCode, "agent error")
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "unloaded"})
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
		Model      string `json:"model"`
		NodeID     string `json:"node_id"`
		Banned     bool   `json:"is_banned"`
		Pinned     bool   `json:"is_pinned"`
		Persistent bool   `json:"is_persistent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if err := b.Storage.SetModelPolicy(req.Model, req.NodeID, req.Banned, req.Pinned, req.Persistent); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger immediate cache refresh
	b.refreshCaches()

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "policy updated"})
}

func (b *Balancer) HandleV1UserModelPolicySet(w http.ResponseWriter, r *http.Request) {
	var p models.UserModelPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if p.UserID == "" {
		b.jsonError(w, http.StatusBadRequest, "user_id required")
		return
	}

	if err := b.Storage.SetUserModelPolicy(p); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "user policy updated"})
}

func (b *Balancer) HandleV1UserModelPoliciesList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	policies, err := b.Storage.ListUserModelPolicies(id)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, policies)
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
		k.Key = "ak_" + b.computeHash(fmt.Sprintf("%d_%s_key", time.Now().UnixNano(), k.Label))[:24]
	}
	if k.BalancerToken == "" {
		k.BalancerToken = "bt_" + b.computeHash(fmt.Sprintf("%d_%s_bt", time.Now().UnixNano(), k.Label))[:24]
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
		aks, _ := b.Storage.GetAgentKeysByUserID(u.ID)
		var agentEarnings float64
		for _, ak := range aks {
			agentEarnings += ak.CreditsEarned
		}
		res = append(res, models.UserWithKey{User: u, Key: key, AgentEarnings: agentEarnings})
	}
	b.jsonResponse(w, http.StatusOK, res)
}

func (b *Balancer) HandleV1UserUpdateQuota(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		QuotaTier         *string `json:"quota_tier,omitempty"`
		QuotaLimit        *int64  `json:"quota_limit,omitempty"`
		DailyQuotaLimit   *int64  `json:"daily_quota_limit,omitempty"`
		WeeklyQuotaLimit  *int64  `json:"weekly_quota_limit,omitempty"`
		MonthlyQuotaLimit *int64  `json:"monthly_quota_limit,omitempty"`
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
	if req.QuotaTier != nil {
		tier := models.QuotaTier(*req.QuotaTier)
		if limits, ok := models.DefaultTiers[tier]; ok {
			u.QuotaTier = tier
			u.QuotaLimit = limits.Total
			u.DailyQuotaLimit = limits.Daily
			u.WeeklyQuotaLimit = limits.Weekly
			u.MonthlyQuotaLimit = limits.Monthly
		}
	}
	if req.QuotaLimit != nil {
		u.QuotaLimit = *req.QuotaLimit
	}
	if req.DailyQuotaLimit != nil {
		u.DailyQuotaLimit = *req.DailyQuotaLimit
	}
	if req.WeeklyQuotaLimit != nil {
		u.WeeklyQuotaLimit = *req.WeeklyQuotaLimit
	}
	if req.MonthlyQuotaLimit != nil {
		u.MonthlyQuotaLimit = *req.MonthlyQuotaLimit
	}
	if req.QuotaTier == nil || models.QuotaTier(*req.QuotaTier) == models.QuotaTierCustom {
		u.QuotaTier = models.QuotaTierCustom
	}
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

func (b *Balancer) HandleV1AgentKeyRotate(w http.ResponseWriter, r *http.Request) {
	oldKey := chi.URLParam(r, "key")

	var req struct {
		RotateAgentToken    bool `json:"rotate_agent_token"`
		RotateBalancerToken bool `json:"rotate_balancer_token"`
	}
	// Default: rotate both
	req.RotateAgentToken = true
	req.RotateBalancerToken = true
	json.NewDecoder(r.Body).Decode(&req)

	existing, err := b.Storage.GetAgentKey(oldKey)
	if err != nil {
		b.jsonError(w, http.StatusNotFound, "agent key not found")
		return
	}

	newKey := oldKey
	if req.RotateAgentToken {
		newKey = "ak_" + b.computeHash(fmt.Sprintf("%d_%s_rotated_key", time.Now().UnixNano(), existing.Label))[:24]
	}

	newBalancerToken := existing.BalancerToken
	if req.RotateBalancerToken {
		newBalancerToken = "bt_" + b.computeHash(fmt.Sprintf("%d_%s_rotated_bt", time.Now().UnixNano(), existing.Label))[:24]
	}

	updated, err := b.Storage.RotateAgentKey(oldKey, newKey, newBalancerToken)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to rotate key: "+err.Error())
		return
	}

	// Update in-flight cluster state so balancer uses the new balancer token immediately
	b.State.Do(func(s *ClusterState) {
		for _, node := range s.Agents {
			if node.AgentKey == oldKey {
				node.AgentKey = newKey
				node.BalancerToken = newBalancerToken
				break
			}
		}
	})

	logging.Global.Infof("Agent key rotated: %s -> %s (label: %s)", oldKey, newKey, existing.Label)
	b.jsonResponse(w, http.StatusOK, updated)
}

func (b *Balancer) HandleV1AgentKeyDelete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := b.Storage.DeleteAgentKey(key); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── User-scoped key management (ownership enforced, no admin required) ───────

func (b *Balancer) requireUser(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	val := r.Context().Value(auth.ContextKeyUser)
	if val == nil {
		b.jsonError(w, http.StatusUnauthorized, "authentication required")
		return models.User{}, false
	}
	u, ok := val.(models.User)
	if !ok {
		b.jsonError(w, http.StatusUnauthorized, "authentication required")
		return models.User{}, false
	}
	return u, true
}

func (b *Balancer) HandleV1UserClientKeyCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	var k models.ClientKey
	if err := json.NewDecoder(r.Body).Decode(&k); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k.UserID = user.ID
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

func (b *Balancer) HandleV1UserClientKeyDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	k, err := b.Storage.GetClientKey(key)
	if err != nil || k.UserID != user.ID {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	if err := b.Storage.DeleteClientKey(key); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1UserClientKeyUpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	k, err := b.Storage.GetClientKey(key)
	if err != nil || k.UserID != user.ID {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	var req struct {
		ErrorFormat string `json:"error_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k.ErrorFormat = req.ErrorFormat
	if err := b.Storage.UpdateClientKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, k)
}

func (b *Balancer) HandleV1UserAgentKeyCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	var k models.AgentKey
	if err := json.NewDecoder(r.Body).Decode(&k); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k.UserID = user.ID
	if k.Key == "" {
		k.Key = "ak_" + b.computeHash(fmt.Sprintf("%d_%s_key", time.Now().UnixNano(), k.Label))[:24]
	}
	if k.BalancerToken == "" {
		k.BalancerToken = "bt_" + b.computeHash(fmt.Sprintf("%d_%s_bt", time.Now().UnixNano(), k.Label))[:24]
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

func (b *Balancer) HandleV1UserAgentKeyDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	k, err := b.Storage.GetAgentKey(key)
	if err != nil || k.UserID != user.ID {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	if err := b.Storage.DeleteAgentKey(key); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (b *Balancer) HandleV1UserAgentKeyRotate(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	oldKey := chi.URLParam(r, "key")
	existing, err := b.Storage.GetAgentKey(oldKey)
	if err != nil || existing.UserID != user.ID {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}

	var req struct {
		RotateAgentToken    bool `json:"rotate_agent_token"`
		RotateBalancerToken bool `json:"rotate_balancer_token"`
	}
	req.RotateAgentToken = true
	req.RotateBalancerToken = true
	json.NewDecoder(r.Body).Decode(&req)

	newKey := oldKey
	if req.RotateAgentToken {
		newKey = "ak_" + b.computeHash(fmt.Sprintf("%d_%s_rotated_key", time.Now().UnixNano(), existing.Label))[:24]
	}
	newBalancerToken := existing.BalancerToken
	if req.RotateBalancerToken {
		newBalancerToken = "bt_" + b.computeHash(fmt.Sprintf("%d_%s_rotated_bt", time.Now().UnixNano(), existing.Label))[:24]
	}

	updated, err := b.Storage.RotateAgentKey(oldKey, newKey, newBalancerToken)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to rotate key: "+err.Error())
		return
	}
	b.State.Do(func(s *ClusterState) {
		for _, node := range s.Agents {
			if node.AgentKey == oldKey {
				node.AgentKey = newKey
				node.BalancerToken = newBalancerToken
				break
			}
		}
	})
	b.jsonResponse(w, http.StatusOK, updated)
}

func (b *Balancer) HandleV1UserAgentKeyUpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	k, err := b.Storage.GetAgentKey(key)
	if err != nil || k.UserID != user.ID {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	var req struct {
		ModelVisibility string `json:"model_visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k.ModelVisibility = req.ModelVisibility
	if err := b.Storage.UpdateAgentKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, k)
}

// ─────────────────────────────────────────────────────────────────────────────

func (b *Balancer) HandleV1UserDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.DeleteUser(id); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleV1UserUpdateSettings lets the authenticated user update their own routing preference.
func (b *Balancer) HandleV1UserUpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := b.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		RoutePreference string `json:"route_preference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.RoutePreference != "" && body.RoutePreference != "quality" && body.RoutePreference != "quality_fallback" {
		b.jsonError(w, http.StatusBadRequest, "route_preference must be '', 'quality', or 'quality_fallback'")
		return
	}
	if err := b.Storage.UpdateUserRoutePreference(user.ID, body.RoutePreference); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"route_preference": body.RoutePreference})
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

func (b *Balancer) HandleV1TestInference(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model  string `json:"model"`
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// 1. Select Target Agent
	var agentAddr string
	b.State.View(func(s ClusterState) {
		for addr, n := range s.Agents {
			if n.ID == req.NodeID {
				agentAddr = addr
				break
			}
		}
	})

	if agentAddr == "" {
		b.jsonError(w, http.StatusNotFound, "target node not found")
		return
	}

	// 2. Prepare test request
	testPrompt := models.InferenceRequest{
		Model:  req.Model,
		Prompt: "Why is the sky blue?",
		Stream: false,
	}
	body, _ := json.Marshal(testPrompt)

	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)
	resp, err := b.sendToAgentWithContext(r.Context(), agentAddr, "/api/generate", body)
	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	// Capture usage
	clientKey, _ := auth.GetTokenFromContext(r.Context())
	var userID string
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok {
			userID = u.ID
		}
	}
	go b.captureUsage(req.NodeID, req.Model, 100, 100, 0, 0, clientKey, userID, surge)

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		b.jsonError(w, resp.StatusCode, string(bodyBytes))
		return
	}

	var result models.InferenceResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to decode response: "+string(bodyBytes))
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"agent_id": req.NodeID,
		"response": result.Response,
	})
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

func openAIErrorType(code int) string {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return "authentication_error"
	case code == http.StatusBadRequest:
		return "invalid_request_error"
	case code == http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "api_error"
	}
}

func (b *Balancer) respondError(w http.ResponseWriter, r *http.Request, code int, message string) {
	var format string
	if ck, ok := auth.GetClientDataFromContext(r.Context()); ok {
		format = ck.ErrorFormat
	}
	if format == "openai" {
		b.jsonResponse(w, code, map[string]interface{}{
			"error": map[string]interface{}{
				"message": message,
				"type":    openAIErrorType(code),
				"param":   nil,
				"code":    nil,
			},
		})
		return
	}
	b.jsonError(w, code, message)
}

func (b *Balancer) HandleV1ClientKeyUpdateSettings(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req struct {
		ErrorFormat string `json:"error_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k, err := b.Storage.GetClientKey(key)
	if err != nil {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	k.ErrorFormat = req.ErrorFormat
	if err := b.Storage.UpdateClientKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, k)
}

func (b *Balancer) HandleV1AgentKeyUpdateSettings(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req struct {
		ModelVisibility string `json:"model_visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request")
		return
	}
	k, err := b.Storage.GetAgentKey(key)
	if err != nil {
		b.jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	k.ModelVisibility = req.ModelVisibility
	if err := b.Storage.UpdateAgentKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, k)
}

func (b *Balancer) jsonResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

func (b *Balancer) jsonError(w http.ResponseWriter, code int, message string) {
	b.jsonResponse(w, code, map[string]string{"error": message})
}
