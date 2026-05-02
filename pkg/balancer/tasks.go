package balancer

import (
	models2 "FlakyOllama/pkg/balancer/models"
	"FlakyOllama/pkg/balancer/queue"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
)

func (b *Balancer) StartLogBroadcaster() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case entry := <-b.LogCh:
				go func(e logging.LogEntry) {
					if err := b.Storage.RecordLog(e.NodeID, string(e.Level), e.Component, e.Message); err != nil {
						logging.Global.Debugf("Failed to record log to DB: %v", err)
					}
				}(entry)

				data, _ := json.Marshal(entry)
				msg := string(data)
				b.broadcastLog(msg)
			case <-ticker.C:
				heartbeat, _ := json.Marshal(logging.LogEntry{
					Timestamp: time.Now(),
					NodeID:    "balancer",
					Level:     logging.LevelDebug,
					Component: "system",
					Message:   "heartbeat",
				})
				b.broadcastLog(string(heartbeat))
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) broadcastLog(msg string) {
	b.logMu.Lock()
	defer b.logMu.Unlock()
	for ch := range b.logChs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Balancer) StartMetricProcessor() {
	go func() {
		tokenBatch := make([]tokenUsageEntry, 0, 50)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case m := <-b.MetricCh:
				b.Storage.RecordMetric(m.nodeID, m.model, m.latency, m.success)
			case t := <-b.TokenCh:
				tokenBatch = append(tokenBatch, t)
				if len(tokenBatch) >= 50 {
					b.flushTokenBatch(tokenBatch)
					tokenBatch = tokenBatch[:0]
				}
			case <-ticker.C:
				if len(tokenBatch) > 0 {
					b.flushTokenBatch(tokenBatch)
					tokenBatch = tokenBatch[:0]
				}
			case <-b.stopCh:
				if len(tokenBatch) > 0 {
					b.flushTokenBatch(tokenBatch)
				}
				return
			}
		}
	}()
}

func (b *Balancer) flushTokenBatch(batch []tokenUsageEntry) {
	entries := make([]struct {
		NodeID, Model, ClientKey, UserID string
		Input, Output                    int
		Reward, Cost                     float64
		TTFT, Duration                   int64
		SelfServed                       bool
	}, len(batch))

	for i, t := range batch {
		entries[i] = struct {
			NodeID, Model, ClientKey, UserID string
			Input, Output                    int
			Reward, Cost                     float64
			TTFT, Duration                   int64
			SelfServed                       bool
		}{t.nodeID, t.model, t.clientKey, t.userID, t.input, t.output, t.reward, t.cost, t.ttft, t.duration, t.selfServed}
	}

	if err := b.Storage.RecordTokenUsageBatch(entries); err != nil {
		logging.Global.Errorf("Failed to record token usage batch: %v", err)
	}
}

func (b *Balancer) StartPerfCacheRefresher() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		// Initial refresh
		b.refreshCaches()
		for {
			select {
			case <-ticker.C:
				b.refreshCaches()
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) refreshCaches() {
	// 1. Refresh Performance Analytics
	analytics, err := b.Storage.GetPerformanceAnalytics()
	if err == nil {
		b.cacheMu.Lock()
		b.perfCache = make(map[string]struct {
			AvgTTFT, AvgDuration float64
			Requests             int
		})
		for m, a := range analytics {
			b.perfCache[m] = struct {
				AvgTTFT, AvgDuration float64
				Requests             int
			}{
				AvgTTFT:     a.AvgTTFT,
				AvgDuration: a.AvgDuration,
				Requests:    a.Requests,
			}
		}
		b.cacheMu.Unlock()
	}

	// 2. Refresh Cluster Policies
	policies, err := b.Storage.GetModelPolicies()
	if err == nil {
		b.cacheMu.Lock()
		b.policyCache = policies
		b.cacheMu.Unlock()
	}
}

func (b *Balancer) StartBackgroundTasks() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		maintenanceTicker := time.NewTicker(1 * time.Hour)
		defer maintenanceTicker.Stop()

		for {
			select {
			case <-ticker.C:
				b.ProcessQueue()
				b.ProcessBackgroundRequests()
				b.pollAgentTasks()
				b.CleanupStaleAgents()
			case <-maintenanceTicker.C:
				b.RunMaintenance()
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) ProcessQueue() {
	if b.Queue.QueueDepth() == 0 {
		return
	}

	snap := b.State.GetSnapshot()

	// Quick check if we have ANY healthy nodes at all
	hasHealthy := false
	for _, a := range snap.Agents {
		if a.State == models.StateHealthy && !a.Draining {
			hasHealthy = true
			break
		}
	}
	if !hasHealthy {
		return
	}

	// Process a limited number of requests per tick to prevent thundering herd
	// and allow state to update between assignments.
	maxToProcess := 10
	if depth := b.Queue.QueueDepth(); depth < maxToProcess {
		maxToProcess = depth
	}

	for i := 0; i < maxToProcess; i++ {
		req := b.Queue.Pop()
		if req == nil {
			break
		}

		resolvedModel := b.resolveVirtualModel(req.Request.Model)
		addr, err := b.SelectAgent(resolvedModel, req.UserID, req.IsAdmin, req.ForceOwnNode)
		if err != nil {
			// If we couldn't find a node right now, put it back
			b.Queue.Requeue(req)
			// If SelectAgent failed, it's likely all nodes are saturated for this model,
			// so stop processing for this tick to avoid spinning.
			break
		}

		// Non-blocking send to the waiting request
		select {
		case req.Response <- queue.QueuedResponse{AgentAddr: addr, ResolvedModel: resolvedModel}:
		default:
			// If no one is listening anymore (client disconnected), decrement the workload
			b.decrementWorkload(addr)
		}
	}
}

func (b *Balancer) decrementWorkload(addr string) {
	b.State.Do(func(s *ClusterState) {
		if s.NodeWorkloads[addr] > 0 {
			s.NodeWorkloads[addr]--
		}
	})
}

func (b *Balancer) ProcessBackgroundRequests() {
	reqs, err := b.Storage.ListModelRequests(models2.StatusApproved)
	if err != nil || len(reqs) == 0 {
		return
	}

	for _, r := range reqs {
		logging.Global.Infof("Processing background model request: %s %s on %s", r.Type, r.Model, r.NodeID)

		// Map to correct endpoint (using the new Agent paths)
		path := "/api/models/pull"
		if r.Type == models2.RequestDelete {
			path = "/api/models/delete"
		}

		body, _ := json.Marshal(map[string]string{"model": r.Model})

		if r.NodeID != "" {
			var addr string
			b.State.View(func(s ClusterState) {
				for a, n := range s.Agents {
					if n.ID == r.NodeID {
						addr = a
						break
					}
				}
			})
			if addr != "" {
				// Transition to Processing immediately
				b.Storage.UpdateModelRequestStatus(r.ID, models2.StatusProcessing)
				b.Jobs.UpdateJob(r.ID, JobRunning, 10, "Request sent to agent")

				go func(requestID, a, p string, d []byte) {
					resp, err := b.sendToAgent(a, p, d)
					if err == nil {
						var result struct {
							TaskID string `json:"task_id"`
						}
						if json.NewDecoder(resp.Body).Decode(&result) == nil && result.TaskID != "" {
							b.Storage.UpdateModelRequestTaskID(requestID, result.TaskID)
						}
						resp.Body.Close()
					} else {
						// On failure, revert to approved so it can be retried
						b.Storage.UpdateModelRequestStatus(requestID, models2.StatusApproved)
						b.Jobs.UpdateJob(requestID, JobFailed, 0, "Failed to reach agent: "+err.Error())
					}
				}(r.ID, addr, path, body)
			}
		} else {
			b.Storage.UpdateModelRequestStatus(r.ID, models2.StatusFailed)
			b.Jobs.UpdateJob(r.ID, JobFailed, 0, "No target node specified")
		}
	}
}

func (b *Balancer) pollAgentTasks() {
	reqs, err := b.Storage.ListModelRequests(models2.StatusProcessing)
	if err != nil || len(reqs) == 0 {
		return
	}

	for _, r := range reqs {
		if r.AgentTaskID == "" {
			continue
		}

		// Find agent address
		var addr string
		var balancerToken string
		b.State.View(func(s ClusterState) {
			for a, n := range s.Agents {
				if n.ID == r.NodeID {
					addr = a
					balancerToken = n.BalancerToken
					break
				}
			}
		})

		if addr == "" {
			continue
		}

		go func(req models2.ModelRequest, agentAddr string, bToken string) {
			scheme := "http"
			if b.Config.TLS.Enabled {
				scheme = "https"
			}
			url := fmt.Sprintf("%s://%s/tasks", scheme, agentAddr)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			hReq, _ := http.NewRequestWithContext(ctx, "GET", url, nil)

			if bToken == "" {
				bToken = b.Config.RemoteToken
			}
			if bToken != "" {
				hReq.Header.Set("Authorization", "Bearer "+bToken)
			}

			resp, err := b.httpClient.Do(hReq)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			var tasks []models.AgentTask
			if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
				return
			}

			for _, t := range tasks {
				if t.ID == req.AgentTaskID {
					if t.Status == models.TaskCompleted {
						b.Storage.UpdateModelRequestStatus(req.ID, models2.StatusCompleted)
						b.Jobs.UpdateJob(req.ID, JobCompleted, 100, "Task completed by agent")

						// Force a telemetry poll to update local models list
						go b.pollAgent(agentAddr)
					} else if t.Status == models.TaskFailed {
						b.Storage.UpdateModelRequestStatus(req.ID, models2.StatusFailed)
						b.Jobs.UpdateJob(req.ID, JobFailed, 0, "Agent task failed: "+t.Error)
					}
					break
				}
			}
		}(r, addr, balancerToken)
	}
}

func (b *Balancer) StartTelemetryPoller() {
	go func() {
		ticker := time.NewTicker(time.Duration(b.Config.PollIntervalMs) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.pollAllAgents()
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) pollAllAgents() {
	var agents []string
	b.State.View(func(s ClusterState) {
		for addr := range s.Agents {
			agents = append(agents, addr)
		}
	})

	for _, addr := range agents {
		go b.pollAgent(addr)
	}
}

func (b *Balancer) pollAgent(addr string) {
	scheme := "http"
	if b.Config.TLS.Enabled {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/telemetry", scheme, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var balancerToken string
	b.State.View(func(s ClusterState) {
		if agent, ok := s.Agents[addr]; ok {
			balancerToken = agent.BalancerToken
		}
	})
	if balancerToken == "" {
		balancerToken = b.Config.RemoteToken
	}
	logging.Global.Debugf("pollAgent %s: using balancerToken=%q (fell_back_to_remote=%v)", addr, balancerToken, balancerToken == b.Config.RemoteToken && b.Config.RemoteToken != "")

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if balancerToken != "" {
		req.Header.Set("Authorization", "Bearer "+balancerToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		logging.Global.Debugf("Telemetry request to %s failed: %v", addr, err)
		b.recordError(addr, "telemetry_failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logging.Global.Debugf("Telemetry to %s rejected with status %d", addr, resp.StatusCode)
		b.recordError(addr, fmt.Sprintf("telemetry_status_%d", resp.StatusCode))
		return
	}

	var status models.NodeStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		logging.Global.Debugf("Failed to decode telemetry from %s: %v", addr, err)
		return
	}

	// Identify persistent models for this node
	var persistentModels []string
	b.cacheMu.RLock()
	for model, nodePolicies := range b.policyCache {
		if pol, ok := nodePolicies[status.ID]; ok && pol.Persistent {
			persistentModels = append(persistentModels, model)
		}
	}
	b.cacheMu.RUnlock()

	b.State.Do(func(s *ClusterState) {
		if existing, ok := s.Agents[addr]; ok {
			// Preserve identity & auth fields — the agent never sends these back in telemetry
			status.ID = existing.ID
			status.AgentKey = existing.AgentKey
			status.BalancerToken = existing.BalancerToken
			status.UserID = existing.UserID
			status.IsGlobal = existing.IsGlobal
			// Preserve connection metadata
			status.Address = existing.Address
			status.Tier = existing.Tier
			// Update liveness fields
			status.LastSeen = time.Now()

			// Only reset to Healthy if we aren't currently Broken and cooling off
			if existing.State == models.StateBroken && existing.CooloffUntil.After(time.Now()) {
				status.State = models.StateBroken
				status.CooloffUntil = existing.CooloffUntil
				status.Message = existing.Message
			} else {
				status.State = models.StateHealthy // It responded to telemetry
				status.Errors = 0                  // Reset errors on successful poll
				status.Message = "Ready"
			}

			s.Agents[addr] = &status
		}
	})
}

func (b *Balancer) CleanupStaleAgents() {
	// Stale threshold is 5 minutes
	threshold := time.Now().Add(-5 * time.Minute)
	b.State.Do(func(s *ClusterState) {
		for addr, a := range s.Agents {
			if a.LastSeen.Before(threshold) {
				logging.Global.Infof("Cleaning up stale node: %s (%s) last seen %v", a.ID, addr, a.LastSeen)
				delete(s.Agents, addr)
				delete(s.NodeWorkloads, addr)
			}
		}
	})
}

func (b *Balancer) RunMaintenance() {
	logging.Global.Infof("Running database maintenance...")

	// 1. Prune metrics older than 7 days
	if err := b.Storage.PruneMetrics(7); err != nil {
		logging.Global.Warnf("Maintenance: Failed to prune metrics: %v", err)
	}

	// 2. Prune logs older than 1000 entries (default for now)
	if err := b.Storage.PruneLogs(1000); err != nil {
		logging.Global.Warnf("Maintenance: Failed to prune logs: %v", err)
	}

	// 3. Normalize reputations periodically to prevent permanent score drift
	if err := b.Storage.NormalizeReputation(0.01); err != nil {
		logging.Global.Warnf("Maintenance: Failed to normalize reputation: %v", err)
	}
}
