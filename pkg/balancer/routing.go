package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

func (b *Balancer) SelectAgent(modelName, userID string) (string, error) {
	var bestAgent string
	var bestScore float64 = -1

	// Strip common prefixes for compatibility
	modelName = strings.TrimPrefix(modelName, "a.")

	// 1. Check User-Specific Policy
	if userID != "" {
		p, err := b.Storage.GetUserModelPolicy(userID, modelName)
		if err == nil && p.Disabled {
			return "", fmt.Errorf("user not authorized to use model %s", modelName)
		}
	}

	snap := b.State.GetSnapshot()

	// 2. Initial Filter
	candidates := make([]string, 0)

	b.cacheMu.RLock()
	clusterPolicies := b.policyCache[modelName]
	b.cacheMu.RUnlock()

	for addr, a := range snap.Agents {
		if a.State == models.StateBroken || a.Draining {
			continue
		}
		if a.CooloffUntil.After(time.Now()) {
			continue
		}

		// Check Cluster-wide Policy for this node/model
		if pol, ok := clusterPolicies[a.ID]; ok && pol.Banned {
			continue
		}

		// Satiation check: Skip nodes with too many active workloads relative to cores
		maxWorkloads := a.CPUCores * 2
		if a.HasGPU {
			maxWorkloads = a.CPUCores * 4
		}
		if snap.NodeWorkloads[addr] >= maxWorkloads {
			continue
		}

		candidates = append(candidates, addr)
	}

	if len(candidates) == 0 {
		// If everything is saturated, fallback to the least loaded healthy nodes
		for addr, a := range snap.Agents {
			if a.State == models.StateHealthy && !a.Draining {
				if pol, ok := clusterPolicies[a.ID]; ok && pol.Banned {
					continue
				}
				candidates = append(candidates, addr)
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no available or healthy nodes for model %s", modelName)
	}

	// 3. Forced Pinning check
	for _, a := range snap.Agents {
		if pol, ok := clusterPolicies[a.ID]; ok && pol.Pinned {
			pinnedCandidates := make([]string, 0)
			for _, c := range candidates {
				if snap.Agents[c].ID == a.ID {
					pinnedCandidates = append(pinnedCandidates, c)
				}
			}
			if len(pinnedCandidates) > 0 {
				candidates = pinnedCandidates
				break
			}
		}
	}

	// 4. Enhanced Scoring
	for _, addr := range candidates {
		a := snap.Agents[addr]

		// Base score from reputation
		score := a.Reputation * 100.0

		// Penalize workload
		workload := float64(snap.NodeWorkloads[addr])
		score -= (workload * b.Config.Weights.WorkloadPenalty * 50.0)

		// Bonus for locally available model
		hasModel := false
		for _, m := range a.LocalModels {
			if m.Name == modelName {
				hasModel = true
				break
			}
		}
		if hasModel {
			score += b.Config.Weights.LocalModelBonus * 25.0
		}

		// Hardware penalties (CPU / Memory saturation)
		if a.CPUUsage > 80 {
			score -= (a.CPUUsage - 80) * 2.0
		}
		if a.MemoryUsage > 90 {
			score -= (a.MemoryUsage - 90) * 5.0
		}

		// VRAM check (if node has GPU)
		if a.HasGPU && a.VRAMTotal > 0 {
			vramUsedPercent := (float64(a.VRAMUsed) / float64(a.VRAMTotal)) * 100.0
			if vramUsedPercent > 85 {
				score -= (vramUsedPercent - 85) * 4.0
			}
		} else if a.HasGPU {
			score -= 10.0
		}

		if score > bestScore {
			bestScore = score
			bestAgent = addr
		}
	}

	if bestAgent == "" {
		bestAgent = candidates[rand.Intn(len(candidates))]
	}

	// 5. Mark workload
	b.State.Do(func(s *ClusterState) {
		s.NodeWorkloads[bestAgent]++
	})

	return bestAgent, nil
}
