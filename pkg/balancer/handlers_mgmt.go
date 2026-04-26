package balancer

import (
	"FlakyOllama/pkg/balancer/jobs"
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/balancer/storage"
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

func (b *Balancer) HandleV1Status(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()
	
	// Enrich with queue depth and other metrics
	resp := struct {
		state.StateSnapshot
		QueueDepth int `json:"queue_depth"`
	}{
		StateSnapshot: snapshot,
		QueueDepth:    b.Queue.QueueDepth(),
	}

	b.jsonResponse(w, http.StatusOK, resp)
}

func (b *Balancer) HandleV1ClusterStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()
	
	var totalVRAM, usedVRAM uint64
	var totalCPU float64
	var nodeCount int
	for _, n := range snapshot.Agents {
		totalVRAM += n.VRAMTotal
		usedVRAM += n.VRAMUsed
		totalCPU += n.CPUUsage
		nodeCount++
	}

	avgCPU := 0.0
	if nodeCount > 0 {
		avgCPU = totalCPU / float64(nodeCount)
	}

	// Create high-level status summary
	type statusSummary struct {
		ActiveWorkloads int                        `json:"active_workloads"`
		AvgCPUUsage     float64                    `json:"avg_cpu_usage"`
		AvgMemUsage     float64                    `json:"avg_mem_usage"`
		TotalVRAM       uint64                     `json:"total_vram"`
		UsedVRAM        uint64                     `json:"used_vram"`
		UptimeSeconds   float64                    `json:"uptime_seconds"`
		QueueDepth      int                        `json:"queue_depth"`
		PendingRequests map[string]int             `json:"pending_requests"`
		AllModels       []string                   `json:"all_models"`
		Nodes           map[string]models.NodeStatus `json:"nodes"`
		ModelPolicies   map[string]map[string]struct{ Banned, Pinned bool } `json:"model_policies"`
	}

	summary := statusSummary{
		ActiveWorkloads: len(snapshot.NodeWorkloads), // Approximation
		AvgCPUUsage:     avgCPU,
		AvgMemUsage:     0, // Approximation
		TotalVRAM:       totalVRAM,
		UsedVRAM:        usedVRAM,
		UptimeSeconds:   time.Since(b.StartTime).Seconds(),
		QueueDepth:      b.Queue.QueueDepth(),
		PendingRequests: snapshot.PendingRequests,
		AllModels:       snapshot.AllModels,
		Nodes:           snapshot.Agents,
		ModelPolicies:   snapshot.ModelPolicies,
	}

	b.jsonResponse(w, http.StatusOK, summary)
}

func (b *Balancer) HandleV1Nodes(w http.ResponseWriter, r *http.Request) {
	snapshot := b.State.GetSnapshot()
	var nodeList []models.NodeStatus
	for _, n := range snapshot.Agents {
		nodeList = append(nodeList, n)
	}
	
	sort.Slice(nodeList, func(i, j int) bool {
		return nodeList[i].ID < nodeList[j].ID
	})

	b.jsonResponse(w, http.StatusOK, nodeList)
}

func (b *Balancer) HandleV1NodeDrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.UpdateNodeByID(id, func(n *models.NodeStatus) {
		n.Draining = true
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "draining initiated"})
}

func (b *Balancer) HandleV1NodeUndrain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.UpdateNodeByID(id, func(n *models.NodeStatus) {
		n.Draining = false
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "node active"})
}

func (b *Balancer) HandleV1NodeDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b.State.Do(func(s *state.ClusterState) {
		for addr, a := range s.Agents {
			if a.ID == id {
				delete(s.Agents, addr)
				break
			}
		}
	})
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "node removed"})
}

func (b *Balancer) HandleV1Logs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// live log streaming logic...
	// (Keeping the streaming logic if needed)
}

func (b *Balancer) HandleV1LogHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	logs, err := b.Storage.GetRecentLogs(limit)
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, logs)
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
	if err := b.Storage.UpdateModelRequestStatus(id, "approved"); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (b *Balancer) HandleV1ModelRequestDecline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := b.Storage.UpdateModelRequestStatus(id, "declined"); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "declined"})
}

func (b *Balancer) HandleV1ModelPolicySet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model   string `json:"model"`
		NodeID  string `json:"node_id"`
		IsBanned bool  `json:"is_banned"`
		IsPinned bool  `json:"is_pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := b.Storage.SetModelPolicy(req.Model, req.NodeID, req.IsBanned, req.IsPinned); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger state refresh
	b.State.Do(func(s *state.ClusterState) {
		policies, _ := b.Storage.GetModelPolicies()
		s.ModelPolicies = policies
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

	isAdmin := false
	// Auto-assign UserID if from OIDC session
	if user, ok := r.Context().Value(auth.ContextKeyUser).(models.User); ok {
		req.UserID = user.ID
		isAdmin = user.IsAdmin
	} else if user, ok := r.Context().Value(auth.ContextKeyUser).(*models.User); ok {
		req.UserID = user.ID
		isAdmin = user.IsAdmin
	}

	if req.Key == "" {
		req.Key = "sk-" + b.computeHash(fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(1000000)))[:32]
	}

	// Moderation check
	if b.Config.EnableKeyApproval && !isAdmin {
		req.Status = models.KeyStatusPending
		req.Active = false
	} else {
		req.Status = models.KeyStatusActive
		req.Active = true
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

	isAdmin := false
	// Auto-assign UserID if from OIDC session
	if user, ok := r.Context().Value(auth.ContextKeyUser).(models.User); ok {
		req.UserID = user.ID
		isAdmin = user.IsAdmin
	} else if user, ok := r.Context().Value(auth.ContextKeyUser).(*models.User); ok {
		req.UserID = user.ID
		isAdmin = user.IsAdmin
	}

	if req.Key == "" {
		req.Key = "ak-" + b.computeHash(fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(1000000)))[:32]
	}

	// Moderation check
	if b.Config.EnableKeyApproval && !isAdmin {
		req.Status = models.KeyStatusPending
		req.Active = false
	} else {
		req.Status = models.KeyStatusActive
		req.Active = true
	}

	if err := b.Storage.CreateAgentKey(req); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b.jsonResponse(w, http.StatusCreated, req)
}

func (b *Balancer) HandleV1KeySetStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type   string           `json:"type"` // "client" or "agent"
		Key    string           `json:"key"`
		Status models.KeyStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	active := req.Status == models.KeyStatusActive
	if err := b.Storage.SetKeyStatus(req.Type, req.Key, req.Status, active); err != nil {
		b.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "updated"})
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
	var user models.User
	var clientKeys []models.ClientKey
	var agentKeys []models.AgentKey

	val := r.Context().Value(auth.ContextKeyUser)
	if u, ok := val.(models.User); ok {
		user = u
	} else if u, ok := val.(*models.User); ok {
		user = *u
	} else {
		// Try token-based auth fallback
		if tkn, ok := r.Context().Value(auth.ContextKeyToken).(string); ok {
			if tkn == b.Config.AuthToken {
				user = models.User{ID: "master", Name: "Master Admin", Email: "admin@local", IsAdmin: true}
				clientKeys = []models.ClientKey{{Key: tkn, Label: "Master Token", Credits: 999999, QuotaLimit: -1, Active: true, Status: models.KeyStatusActive}}
			} else if ck, ok := r.Context().Value(auth.ContextKeyClientData).(models.ClientKey); ok {
				user = models.User{ID: "token-user", Name: ck.Label, Email: "client@token", IsAdmin: false}
				clientKeys = []models.ClientKey{ck}
			}
		}
	}

	if user.ID == "" {
		logging.Global.Warnf("HandleV1Me: User context missing or wrong type. Got: %T", val)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// If we still need to fetch details for OIDC user
	if user.ID != "master" && user.ID != "token-user" {
		// Fetch fresh user to get latest quotas
		u, err := b.Storage.GetUserByID(user.ID)
		if err == nil {
			user = u
		}

		cks, err := b.Storage.GetClientKeysByUserID(user.ID)
		if err != nil || len(cks) == 0 {
			defaultKey := models.ClientKey{
				Key:        "sk-" + b.computeHash(fmt.Sprintf("%s-default", user.ID))[:32],
				Label:      fmt.Sprintf("Personal Key for %s", user.Name),
				QuotaLimit: 1000000,
				QuotaUsed:  0,
				Credits:    10.0,
				Active:     true,
				UserID:     user.ID,
				Status:     models.KeyStatusActive,
			}
			b.Storage.CreateClientKey(defaultKey)
			clientKeys = []models.ClientKey{defaultKey}
		} else {
			clientKeys = cks
		}

		ak, err := b.Storage.GetAgentKeysByUserID(user.ID)
		if err == nil {
			agentKeys = ak
		}
	}

	resp := struct {
		User       models.User       `json:"user"`
		ClientKeys []models.ClientKey `json:"client_keys"`
		AgentKeys  []models.AgentKey `json:"agent_keys"`
	}{
		User:       user,
		ClientKeys: clientKeys,
		AgentKeys:  agentKeys,
	}

	b.jsonResponse(w, http.StatusOK, resp)
}

func (b *Balancer) HandleV1UsersList(w http.ResponseWriter, r *http.Request) {
	users, err := b.Storage.ListUsers()
	if err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}

	type userWithKey struct {
		User models.User      `json:"user"`
		Key  models.ClientKey `json:"key"`
	}

	var resp []userWithKey
	for _, u := range users {
		cks, _ := b.Storage.GetClientKeysByUserID(u.ID)
		var k models.ClientKey
		if len(cks) > 0 { k = cks[0] }
		resp = append(resp, userWithKey{User: u, Key: k})
	}

	b.jsonResponse(w, http.StatusOK, resp)
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

	cks, err := b.Storage.GetClientKeysByUserID(id)
	if err != nil || len(cks) == 0 {
		b.jsonError(w, http.StatusNotFound, "user has no client key")
		return
	}

	k := cks[0]
	k.QuotaLimit = req.QuotaLimit
	if err := b.Storage.UpdateClientKey(k); err != nil {
		b.jsonError(w, http.StatusInternalServerError, "failed to update quota: "+err.Error())
		return
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "quota updated"})
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

	// Update the live config
	b.configMu.Lock()
	*b.Config = newCfg
	b.configMu.Unlock()

	// Optionally save to disk if CONFIG_PATH is set
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		if err := b.Config.SaveConfig(path); err != nil {
			logging.Global.Errorf("Failed to save config to %s: %v", path, err)
		}
	}

	b.jsonResponse(w, http.StatusOK, map[string]string{"status": "config updated"})
}

func (b *Balancer) HandleV1TestInference(w http.ResponseWriter, r *http.Request) {
	var req models.InferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Model == "" {
		b.jsonError(w, http.StatusBadRequest, "model name required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	// Lock in surge
	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

	body, _ := json.Marshal(req)
	resp, agentID, _, err := b.DoHedgedRequest(ctx, req.Model, "/inference", body, r.RemoteAddr, false, 0, "")

	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	// Capture usage
	clientKey, _ := auth.GetTokenFromContext(r.Context())
	bodyBytes, _ := io.ReadAll(resp.Body)
	go b.captureUsage(agentID, req.Model, bodyBytes, clientKey, 0, 0, surge)


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
