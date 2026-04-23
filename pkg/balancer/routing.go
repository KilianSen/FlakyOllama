package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Route finds the best agent for an inference request using adaptive heuristics and session stickiness.
func (b *Balancer) Route(ctx context.Context, req models.InferenceRequest, clientIP string) (string, string, error) {
	// Clean model name (strip prefixes like a. added by some tools)
	if strings.HasPrefix(req.Model, "a.") {
		req.Model = strings.TrimPrefix(req.Model, "a.")
	}

	// 0. Check User-specific Model Policy (Disabled status)
	userID := ""
	if val := ctx.Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok {
			userID = u.ID
		} else if u, ok := val.(*models.User); ok {
			userID = u.ID
		}
	} else if val := ctx.Value(auth.ContextKeyClientData); val != nil {
		if ck, ok := val.(models.ClientKey); ok {
			userID = ck.UserID
		}
	}

	if userID != "" {
		p, _ := b.Storage.GetUserModelPolicy(userID, req.Model)
		if p.Disabled {
			return "", "", fmt.Errorf("model %s is disabled for this user", req.Model)
		}
	}

	snapshot := b.State.GetSnapshot()
	pending := snapshot.PendingRequests[req.Model]

	b.affinityMu.RLock()
	affinityID := b.ClientAffinity[clientIP]
	b.affinityMu.RUnlock()

	var bestAgent models.NodeStatus
	var bestScore = -1000.0
	var foundBest = false

	// Get model requirements from learned metadata
	minVRAM, _ := b.Storage.GetModelVRAM(req.Model)
	if minVRAM == 0 {
		minVRAM = estimateVRAMFallback(req.Model)
	}

	foundLoaded := false
	now := time.Now()

	for addr, a := range snapshot.Agents {
		// 0. Policy Check: Is model banned on this node?
		if policy, ok := snapshot.ModelPolicies[req.Model]; ok {
			if p, ok := policy[a.ID]; ok && p.Banned {
				continue
			}
		}

		// Connectivity and state checks
		if time.Since(a.LastSeen) > 5*time.Second || a.Draining {
			continue
		}
		if a.State == models.StateBroken && now.Before(a.CooloffUntil) {
			continue
		}
		if a.VRAMTotal < minVRAM {
			// If not enough VRAM, check if we can run on CPU (System RAM)
			// We allow a small buffer for OS/other processes
			if a.MemoryTotal < minVRAM+(1024*1024*1024) {
				continue
			}
			// Node qualifies via CPU, but will get a lower score later
		}

		// Use Performance Cache
		b.perfMu.RLock()
		perf, ok := b.PerfCache[a.ID+":"+req.Model]
		b.perfMu.RUnlock()

		if !ok {
			perf = storage.PerformanceMetric{SuccessRate: 1.0, AvgLatency: 1.0}
		}

		// 1. Foundation: CPU Load (Inverse)
		score := (1.0 - (a.CPUUsage / 100.0)) * b.Config.Weights.CPULoadWeight

		// 2. Least Connections: Penalize nodes with active workloads to prevent thundering herd
		workload := snapshot.NodeWorkloads[addr]
		score -= float64(workload) * b.Config.Weights.WorkloadPenalty

		// 3. Thermal Protection
		if a.GPUTemperature > 80.0 {
			score *= 0.5
		}
		if a.GPUTemperature > 90.0 {
			continue // Critical thermal threshold
		}

		// 4. Historical Reliability
		successRate := perf.SuccessRate
		if successRate <= 0 {
			successRate = 1.0
		}
		score *= successRate * b.Config.Weights.SuccessRateWeight

		if perf.AvgLatency > 0 {
			score *= (1.0 / perf.AvgLatency) * b.Config.Weights.LatencyWeight
		}

		// 4.1 Reputation System (Ecosystem Penalty/Bonus)
		if a.Reputation > 0 {
			score *= a.Reputation
		}

		// 5. Degradation Penalty
		if a.State == models.StateDegraded {
			score *= 0.5
		}

		// 6. Node Tiering: Penalize shared nodes so they are only used as overflow
		if a.Tier == "shared" {
			score -= 10.0
		}

		// 7. Session Stickiness: Grant bonus for KV Cache locality
		if a.ID == affinityID {
			score += 2.0
		}

		// 8. Model Residency (Hot vs Warm vs Cold)
		isHot := false
		for _, m := range a.ActiveModels {
			if m == req.Model {
				isHot = true
				foundLoaded = true
				break
			}
		}

		if isHot {
			score += b.Config.Weights.LoadedModelBonus
		} else {
			// Check if model is on disk (Warm)
			isWarm := false
			for _, mInfo := range a.LocalModels {
				if mInfo.Model == req.Model {
					isWarm = true
					break
				}
			}

			if isWarm {
				score += b.Config.Weights.LocalModelBonus
				freeVRAM := a.VRAMTotal - a.VRAMUsed
				if freeVRAM < minVRAM {
					score -= 1.0 // Eviction penalty
				}
			} else {
				score -= 5.0
			}
		}

		if score > bestScore {
			bestScore = score
			bestAgent = a
			foundBest = true
		}
	}

	if foundBest {
		b.State.DoAsync(func(s *state.ClusterState) {
			s.NodeWorkloads[bestAgent.Address]++
		})
		b.affinityMu.Lock()
		b.ClientAffinity[clientIP] = bestAgent.ID
		b.affinityMu.Unlock()
	}

	// Auto-allocation logic
	if !foundLoaded || pending > b.Config.StaleThreshold {
		b.triggerAllocation(req.Model, minVRAM)
	}

	if !foundBest {
		logging.Global.Warnf("Routing failed: No suitable agent found for model %s (pending: %d)", req.Model, pending)
		return "", "", fmt.Errorf("no available agents with sufficient capabilities")
	}

	logging.Global.Infof("Routed model %s to agent %s (score: %.2f, pending: %d, affinity: %v)", req.Model, bestAgent.ID, bestScore, pending, bestAgent.ID == affinityID)
	return bestAgent.ID, bestAgent.Address, nil
}

func estimateVRAMFallback(model string) uint64 {
	model = strings.ToLower(model)
	if strings.Contains(model, "70b") {
		return 40 * 1024 * 1024 * 1024
	}
	if strings.Contains(model, "30b") || strings.Contains(model, "33b") || strings.Contains(model, "34b") {
		return 20 * 1024 * 1024 * 1024
	}
	if strings.Contains(model, "13b") || strings.Contains(model, "14b") {
		return 10 * 1024 * 1024 * 1024
	}
	if strings.Contains(model, "7b") || strings.Contains(model, "8b") {
		return 5 * 1024 * 1024 * 1024
	}
	if strings.Contains(model, "3b") || strings.Contains(model, "4b") {
		return 2 * 1024 * 1024 * 1024
	}
	if strings.Contains(model, "1b") || strings.Contains(model, "2b") {
		return 1536 * 1024 * 1024 // 1.5GB to be safe for 1.3GB models
	}
	return 512 * 1024 * 1024 // 512MB absolute minimum
}

func (b *Balancer) triggerAllocation(model string, minVRAM uint64) {
	var alreadyInProgress bool
	b.State.Do(func(s *state.ClusterState) {
		if startTime, ok := s.InProgressPulls[model]; ok {
			if time.Since(startTime) < 10*time.Minute {
				alreadyInProgress = true
				return
			}
		}
		s.InProgressPulls[model] = time.Now()
	})

	if alreadyInProgress {
		return
	}

	snapshot := b.State.GetSnapshot()
	var bestTarget models.NodeStatus
	var bestScore = -1.0
	var foundTarget = false

	for _, a := range snapshot.Agents {
		// 0. Policy Check
		if policy, ok := snapshot.ModelPolicies[model]; ok {
			if p, ok := policy[a.ID]; ok && p.Banned {
				continue
			}
		}

		// Connectivity and basic requirements
		if time.Since(a.LastSeen) > 5*time.Second || a.State == models.StateBroken || a.Draining {
			continue
		}

		// Capability check: VRAM or System RAM
		if a.VRAMTotal < minVRAM && a.MemoryTotal < minVRAM+(1024*1024*1024) {
			continue
		}

		// Skip if model is already there
		alreadyHas := false
		for _, m := range a.ActiveModels {
			if m == model {
				alreadyHas = true
				break
			}
		}
		if alreadyHas {
			continue
		}
		for _, m := range a.LocalModels {
			if m.Model == model {
				alreadyHas = true
				break
			}
		}
		if alreadyHas {
			continue
		}

		// Allocation Score: prioritize nodes with most free VRAM and lowest CPU
		freeVRAM := float64(a.VRAMTotal - a.VRAMUsed)
		score := (freeVRAM / 1e9) * (1.0 - (a.CPUUsage / 100.0))

		if score > bestScore {
			bestScore = score
			bestTarget = a
			foundTarget = true
		}
	}

	if foundTarget {
		logging.Global.Infof("Triggering auto-allocation of model %s to agent %s (allocation score: %.2f)", model, bestTarget.ID, bestScore)
		body, _ := json.Marshal(map[string]string{"model": model})
		go func(addr, m string, bdy []byte) {
			resp, err := b.sendToAgent(addr, "/models/pull", bdy)
			if err != nil || resp.StatusCode >= 400 {
				if err == nil {
					resp.Body.Close()
				}
				logging.Global.Errorf("Auto-allocation of %s to %s failed, clearing lock", m, addr)
				b.State.DoAsync(func(s *state.ClusterState) {
					delete(s.InProgressPulls, m)
				})
			} else {
				resp.Body.Close()
			}
		}(bestTarget.Address, model, body)
	} else {
		// Clear the lock if we couldn't find a target
		b.State.DoAsync(func(s *state.ClusterState) {
			delete(s.InProgressPulls, model)
		})
	}
}
