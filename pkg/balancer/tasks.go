package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"time"
)

func (b *Balancer) StartLogBroadcaster() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
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
				ticker.Stop()
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
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.refreshPerfCache()
			case <-b.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (b *Balancer) refreshPerfCache() {
	// Not implemented yet
}

func (b *Balancer) StartBackgroundTasks() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.ProcessQueue()
				b.ProcessBackgroundRequests()
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

	for b.Queue.QueueDepth() > 0 {
		req := b.Queue.Pop()
		if req == nil {
			break
		}

		addr, err := b.SelectAgent(req.Request.Model)
		if err != nil {
			req.Response <- QueuedResponse{Err: err}
			continue
		}

		req.Response <- QueuedResponse{AgentAddr: addr}
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

		// If nodeID is empty, broadcast to all or let routing pick?
		// For now, if nodeID is specified, send to that node
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
				go b.sendToAgent(addr, path, body)
			}
		}

		b.Storage.UpdateModelRequestStatus(r.ID, "completed")
		b.Jobs.UpdateJob(r.ID, JobCompleted, 100, "Request processed")
	}
}
