package balancer

import (
	"FlakyOllama/pkg/balancer/storage"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Route finds the best agent for an inference request using adaptive heuristics and session stickiness.
func (b *Balancer) Route(req models.InferenceRequest, clientIP string) (string, string, error) {
	b.Mu.RLock()
	pending := b.PendingRequests[req.Model]
	affinityID := b.ClientAffinity[clientIP]

	var bestAgent *models.NodeStatus
	var bestScore = -1000.0

	// Get model requirements from learned metadata
	minVRAM, _ := b.Storage.GetModelVRAM(req.Model)
	if minVRAM == 0 {
		// Fallback for unknown models
		if strings.Contains(req.Model, "7b") {
			minVRAM = 4 * 1024 * 1024 * 1024
		} else if strings.Contains(req.Model, "70b") {
			minVRAM = 40 * 1024 * 1024 * 1024
		}
	}

	foundLoaded := false
	now := time.Now()
	for _, a := range b.Agents {
		// Connectivity and state checks
		if time.Since(a.LastSeen) > 5*time.Second || a.Draining {
			continue
		}
		if a.State == models.StateBroken && now.Before(a.CooloffUntil) {
			continue
		}
		if a.VRAMTotal < minVRAM {
			continue
		}

		// Use Performance Cache
		b.perfMu.RLock()
		perf, ok := b.PerfCache[a.ID+":"+req.Model]
		b.perfMu.RUnlock()

		if !ok {
			// If no performance data exists, default to healthy assumption
			perf = storage.PerformanceMetric{SuccessRate: 1.0, AvgLatency: 1.0}
		}

		// 1. Foundation: CPU Load (Inverse)
		score := (1.0 - (a.CPUUsage / 100.0)) * b.Config.Weights.CPULoadWeight

		// 2. Least Connections: Penalize nodes with active workloads to prevent thundering herd
		workload := b.NodeWorkloads[a.Address]
		score -= float64(workload) * b.Config.Weights.WorkloadPenalty

		// 3. Thermal Protection
		if a.GPUTemperature > 80.0 {
			score *= 0.5
		}
		if a.GPUTemperature > 90.0 {
			continue // Critical thermal threshold
		}

		// 4. Historical Reliability (with Cold-Start defaults)
		successRate := perf.SuccessRate
		if successRate <= 0 {
			successRate = 1.0 // Assume healthy for new nodes
		}
		score *= successRate * b.Config.Weights.SuccessRateWeight

		if perf.AvgLatency > 0 {
			score *= (1.0 / perf.AvgLatency) * b.Config.Weights.LatencyWeight
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
			score += 2.0 // Stickiness bonus
		}

		// 7. Model Residency (Hot vs Warm vs Cold)
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
				if mInfo.Name == req.Model {
					isWarm = true
					break
				}
			}

			if isWarm {
				score += b.Config.Weights.LocalModelBonus

				// VRAM Fragmentation Check: Penalize if Ollama must evict models
				freeVRAM := a.VRAMTotal - a.VRAMUsed
				if freeVRAM < minVRAM {
					score -= 1.0 // Eviction penalty
				}
			} else {
				// Cold start required (Pulling over network)
				score -= 5.0
			}
		}

		if score > bestScore {
			bestScore = score
			bestAgent = a
		}
	}
	b.Mu.RUnlock()

	if bestAgent != nil {
		b.Mu.Lock()
		b.ClientAffinity[clientIP] = bestAgent.ID
		b.Mu.Unlock()
	}

	// Auto-allocation logic
	if !foundLoaded || pending > b.Config.StaleThreshold {
		b.triggerAllocation(req.Model, minVRAM)
	}

	if bestAgent == nil {
		logging.Global.Warnf("Routing failed: No suitable agent found for model %s (pending: %d)", req.Model, pending)
		return "", "", fmt.Errorf("no available agents with sufficient capabilities")
	}

	logging.Global.Infof("Routed model %s to agent %s (score: %.2f, pending: %d, affinity: %v)", req.Model, bestAgent.ID, bestScore, pending, bestAgent.ID == affinityID)
	return bestAgent.ID, bestAgent.Address, nil
}

func (b *Balancer) triggerAllocation(model string, minVRAM uint64) {
	b.Mu.Lock()
	if startTime, ok := b.InProgressPulls[model]; ok {
		// 10-minute safety timeout for pull lock
		if time.Since(startTime) < 10*time.Minute {
			b.Mu.Unlock()
			return
		}
	}
	b.InProgressPulls[model] = time.Now()
	b.Mu.Unlock()

	b.Mu.RLock()
	var bestTarget *models.NodeStatus
	var bestScore = -1.0

	for _, a := range b.Agents {
		// Connectivity and basic requirements
		if time.Since(a.LastSeen) > 5*time.Second || a.VRAMTotal < minVRAM || a.State == models.StateBroken || a.Draining {
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
			if m.Name == model {
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
		}
	}
	b.Mu.RUnlock()

	if bestTarget != nil {
		logging.Global.Infof("Triggering auto-allocation of model %s to agent %s (allocation score: %.2f)", model, bestTarget.ID, bestScore)
		body, _ := json.Marshal(map[string]string{"model": model})
		go b.sendToAgent(bestTarget.Address, "/models/pull", body)
	}
}
