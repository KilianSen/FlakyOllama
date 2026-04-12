package balancer

import (
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/metrics"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (b *Balancer) StartBackgroundTasks() {
	b.StartPoller()
	b.StartKeepAliveCleaner()
	b.StartPerfCacheRefresher()
	b.StartMetricProcessor()
	b.StartLogProcessor()
	b.StartWorkerPool(10) // 10 workers for routing
}

func (b *Balancer) StartLogProcessor() {
	go func() {
		for {
			select {
			case entry := <-b.LogCh:
				data, _ := json.Marshal(entry)
				msg := string(data)
				b.logMu.Lock()
				for ch := range b.logChs {
					select {
					case ch <- msg:
					default:
					}
				}
				b.logMu.Unlock()
			case <-b.stopCh:
				return
			}
		}
	}()
}

func (b *Balancer) StartMetricProcessor() {
	go func() {
		for {
			select {
			case m := <-b.MetricCh:
				b.Storage.RecordMetric(m.nodeID, m.model, m.latency, m.success)
			case <-b.stopCh:
				return
			}
		}
	}()
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
	b.Mu.RLock()
	// Get unique combinations of node IDs and model names from currently known state
	type entry struct{ nodeID, model string }
	entries := make([]entry, 0)
	for _, a := range b.Agents {
		for _, m := range a.ActiveModels {
			entries = append(entries, entry{a.ID, m})
		}
		for _, m := range a.LocalModels {
			entries = append(entries, entry{a.ID, m.Name})
		}
	}
	b.Mu.RUnlock()

	newCache := make(map[string]storage.PerformanceMetric)
	for _, e := range entries {
		perf, err := b.Storage.GetPerformance(e.nodeID, e.model)
		if err == nil {
			newCache[e.nodeID+":"+e.model] = perf
		}
	}

	b.perfMu.Lock()
	b.PerfCache = newCache
	b.perfMu.Unlock()
}

func (b *Balancer) StartWorkerPool(workers int) {
	for i := 0; i < workers; i++ {
		go b.worker()
	}
}

func (b *Balancer) worker() {
	for {
		select {
		case <-b.stopCh:
			return
		case _, ok := <-b.Queue.Wait():
			if !ok {
				return // Queue closed
			}
			// Drain the queue as much as possible
			for {
				req := b.Queue.Pop()
				if req == nil {
					break
				}

				id, addr, err := b.Route(req.Request, req.ClientIP)
				req.Response <- QueuedResponse{AgentID: id, AgentAddr: addr, Err: err}

				// Check for shutdown between items
				select {
				case <-b.stopCh:
					return
				default:
				}
			}
		}
	}
}

func (b *Balancer) StartKeepAliveCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	pruneTicker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.cleanStaleModels()
			case <-pruneTicker.C:
				if err := b.Storage.PruneOldMetrics(2); err != nil {
					logging.Global.Errorf("Failed to prune old metrics: %v", err)
				}
			case <-b.stopCh:
				ticker.Stop()
				pruneTicker.Stop()
				return
			}
		}
	}()
}

func (b *Balancer) cleanStaleModels() {
	b.lastUsedMu.Lock()
	now := time.Now()
	keepAlive := time.Duration(b.Config.KeepAliveDurationSec) * time.Second

	toUnload := make([]struct{ nodeID, addr, model string }, 0)
	for key, lastTime := range b.ModelLastUsed {
		if now.Sub(lastTime) > keepAlive {
			idx := strings.LastIndex(key, ":")
			if idx == -1 {
				continue
			}
			agentAddr := key[:idx]
			modelName := key[idx+1:]

			b.Mu.RLock()
			if agent, ok := b.Agents[agentAddr]; ok {
				toUnload = append(toUnload, struct{ nodeID, addr, model string }{agent.ID, agent.Address, modelName})
			}
			b.Mu.RUnlock()
			delete(b.ModelLastUsed, key)
		}
	}
	b.lastUsedMu.Unlock()

	for _, item := range toUnload {
		logging.Global.Infof("Unloading stale model %s from agent %s", item.model, item.nodeID)
		body, _ := json.Marshal(map[string]string{"model": item.model})
		b.sendToAgent(item.addr, "/models/unload", body)
	}
}

func (b *Balancer) StartPoller() {
	interval := time.Duration(b.Config.PollIntervalMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.pollAgents()
				metrics.QueueDepth.Set(float64(b.Queue.pq.Len()))
			case <-b.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (b *Balancer) Broadcast(path string, body []byte) {
	b.Mu.RLock()
	agents := make([]*models.NodeStatus, 0, len(b.Agents))
	for _, a := range b.Agents {
		if !a.Draining && a.State != models.StateBroken {
			agents = append(agents, a)
		}
	}
	b.Mu.RUnlock()

	for _, a := range agents {
		go func(addr string, id string) {
			resp, err := b.sendToAgent(addr, path, body)
			if err != nil {
				logging.Global.Errorf("Broadcast to %s (%s) failed: %v", id, addr, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				logging.Global.Warnf("Broadcast to %s (%s) returned status %d", id, addr, resp.StatusCode)
			}
		}(a.Address, a.ID)
	}
}

func (b *Balancer) pollAgents() {
	b.Mu.RLock()
	agents := make([]*models.NodeStatus, 0, len(b.Agents))
	for _, a := range b.Agents {
		agents = append(agents, a)
	}
	b.Mu.RUnlock()

	for _, agent := range agents {
		go func(a *models.NodeStatus) {
			scheme := "http"
			if b.Config.TLS.Enabled {
				scheme = "https"
			}
			req, _ := http.NewRequest("GET", scheme+"://"+a.Address+"/telemetry", nil)
			if b.Config.RemoteToken != "" {
				req.Header.Set("Authorization", "Bearer "+b.Config.RemoteToken)
			}
			// Use internal httpClient with timeout
			resp, err := b.httpClient.Do(req)
			if err != nil {
				logging.Global.Errorf("Failed to poll agent %s (%s): %v", a.ID, a.Address, err)
				b.recordError(a.Address)
				return
			}
			defer resp.Body.Close()

			var status models.NodeStatus
			if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
				b.Mu.Lock()
				currentAgent, ok := b.Agents[a.Address]
				if !ok {
					b.Mu.Unlock()
					return
				}

				// Preserving some internal state
				status.State = currentAgent.State
				status.Errors = currentAgent.Errors
				status.Draining = currentAgent.Draining
				status.LastSeen = time.Now()

				b.Agents[a.Address] = &status

				// Preemptive Draining: If shared node is under host stress but cluster-idle, evict models
				if status.Tier == "shared" && (status.CPUUsage > 85.0 || status.GPUTemperature > 85.0) {
					b.workloadMu.RLock()
					workload := b.NodeWorkloads[a.Address]
					b.workloadMu.RUnlock()
					if workload == 0 {
						for _, m := range status.ActiveModels {
							logging.Global.Infof("Preemptively evicting model %s from shared host %s due to system stress", m, status.ID)
							body, _ := json.Marshal(map[string]string{"model": m})
							go b.sendToAgent(status.Address, "/models/unload", body)
						}
					}
				}

				// Update learned VRAM
				for _, m := range status.ActiveModels {
					if len(status.ActiveModels) == 1 {
						// Heuristic: if only one model is loaded, VRAMUsed is a good estimate
						b.Storage.UpdateModelVRAM(m, status.VRAMUsed)
					}
				}

				// Clear InProgressPulls if the model is now visible on any node (active or local)
				b.pullsMu.Lock()
				for m := range b.InProgressPulls {
					found := false
					for _, am := range status.ActiveModels {
						if am == m {
							found = true
							break
						}
					}
					if !found {
						for _, lm := range status.LocalModels {
							if lm.Name == m {
								found = true
								break
							}
						}
					}
					if found {
						logging.Global.Infof("Model %s discovered on node %s, clearing pull lock", m, a.ID)
						delete(b.InProgressPulls, m)
					}
				}
				b.pullsMu.Unlock()

				// Update metrics
				healthVal := 0.0
				switch status.State {
				case models.StateHealthy:
					healthVal = 2.0
				case models.StateDegraded:
					healthVal = 1.0
				default:
					// panic("unhandled default case")
					healthVal = 0.0
				}
				metrics.NodeHealthStatus.WithLabelValues(a.ID, a.Address).Set(healthVal)

				b.Mu.Unlock()
			} else {
				logging.Global.Errorf("Failed to decode telemetry for agent %s (%s): %v", a.ID, a.Address, err)
			}
		}(agent)
	}
}
