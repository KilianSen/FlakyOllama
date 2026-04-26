package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"fmt"
	"math/rand"
	"time"
)

func (b *Balancer) SelectAgent(modelName, userID string) (string, error) {
	var bestAgent string
	var bestScore float64 = -1

	// 1. Check User-Specific Policy
	if userID != "" {
		p, err := b.Storage.GetUserModelPolicy(userID, modelName)
		if err == nil && p.Disabled {
			return "", fmt.Errorf("user not authorized to use model %s", modelName)
		}
	}

	snap := b.State.GetSnapshot()

	// 2. Get nodes that have the model locally
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

		hasModel := false
		for _, m := range a.LocalModels {
			if m.Name == modelName {
				hasModel = true
				break
			}
		}

		if hasModel {
			candidates = append(candidates, addr)
		}
	}

	// 3. Forced Pinning check
	for _, a := range snap.Agents {
		if pol, ok := clusterPolicies[a.ID]; ok && pol.Pinned {
			// If pinned, we ONLY consider these nodes
			// Filter candidates to only pinned ones
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

	if len(candidates) == 0 {
		// Fallback: any node that is healthy
		for addr, a := range snap.Agents {
			if a.State == models.StateHealthy && !a.Draining {
				// Still respect bans even in fallback
				if pol, ok := clusterPolicies[a.ID]; ok && pol.Banned {
					continue
				}
				candidates = append(candidates, addr)
			}
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no available nodes for model %s", modelName)
	}

	// 4. Simple score-based selection
	for _, addr := range candidates {
		a := snap.Agents[addr]
		score := 1.0 / (float64(snap.NodeWorkloads[addr]) + 1.0)
		score *= a.Reputation

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
