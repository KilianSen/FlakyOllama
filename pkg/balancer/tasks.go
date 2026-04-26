package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"time"
)

func (b *Balancer) StartLogBroadcaster() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case entry := <-b.LogCh:
				go func(e models.LogEntry) {
					if err := b.Storage.RecordLog(e.NodeID, string(e.Level), e.Component, e.Message); err != nil {
						logging.Global.Debugf("Failed to record log to DB: %v", err)
					}
				}(entry)

				data, _ := json.Marshal(entry)
				msg := string(data)
				b.broadcastLog(msg)
			case <-ticker.C:
				heartbeat, _ := json.Marshal(models.LogEntry{
					Timestamp: time.Now(),
					NodeID:    "balancer",
					Level:     models.LevelDebug,
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
	}, len(batch))

	for i, t := range batch {
		entries[i] = struct {
			NodeID, Model, ClientKey, UserID string
			Input, Output                    int
			Reward, Cost                     float64
			TTFT, Duration                   int64
		}{t.nodeID, t.model, t.clientKey, t.userID, t.input, t.output, t.reward, t.cost, t.ttft, t.duration}
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

	// Check for available nodes
	snap := b.State.GetSnapshot()
	availableNodes := 0
	for _, a := range snap.Agents {
		if a.State == models.StateHealthy && !a.Draining {
			availableNodes++
		}
	}
	if availableNodes == 0 {
		return
	}

	// Limit processing to prevent infinite loops
	depth := b.Queue.QueueDepth()
	for i := 0; i < depth; i++ {
		req := b.Queue.Pop()
		if req == nil {
			break
		}

		// Use the context from the queued request to pass user info to SelectAgent if needed
		addr, err := b.SelectAgent(req.Request.Model, req.UserID)
		if err != nil {
			// CRITICAL FIX: Use Requeue to preserve original Response channel
			b.Queue.Requeue(req)
			continue
		}

		// Non-blocking send
		select {
		case req.Response <- QueuedResponse{AgentAddr: addr}:
		default:
			// If channel full or no one listening, it's fine, req was already popped
			// Since SelectAgent already incremented, we MUST decrement
			b.decrementWorkload(addr)
		}
	}
}

func (b *Balancer) ProcessBackgroundRequests() {
	reqs, err := b.Storage.ListModelRequests(models.StatusApproved)
	if err != nil || len(reqs) == 0 {
		return
	}

	for _, r := range reqs {
		logging.Global.Infof("Processing background model request: %s %s on %s", r.Type, r.Model, r.NodeID)

		// Map to correct endpoint
		path := "/api/pull"
		if r.Type == models.RequestDelete {
			path = "/api/delete"
		}

		body, _ := json.Marshal(map[string]string{"name": r.Model})

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
				// Manually increment workload for background task
				b.State.Do(func(s *ClusterState) {
					s.NodeWorkloads[addr]++
				})
				go func(a, p string, d []byte) {
					resp, err := b.sendToAgent(a, p, d)
					if err == nil {
						resp.Body.Close()
					} else {
						// Decrement workload on failure since Close() won't be called
						b.decrementWorkload(a)
					}
				}(addr, path, body)
			}
		}

		b.Storage.UpdateModelRequestStatus(r.ID, "completed")
		b.Jobs.UpdateJob(r.ID, JobCompleted, 100, "Request processed")
	}
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
