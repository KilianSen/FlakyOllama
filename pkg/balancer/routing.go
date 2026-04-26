package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"math/rand"
	"time"
)

func (b *Balancer) SelectAgent(modelName string) (string, error) {
	var bestAgent string
	var bestScore float64 = -1

	snap := b.State.GetSnapshot()

	// 1. Get nodes that have the model locally
	candidates := make([]string, 0)
	for addr, a := range snap.Agents {
		if a.State == models.StateBroken || a.Draining {
			continue
		}
		if a.CooloffUntil.After(time.Now()) {
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

	if len(candidates) == 0 {
		// Fallback: any node that is healthy
		for addr, a := range snap.Agents {
			if a.State == models.StateHealthy && !a.Draining {
				candidates = append(candidates, addr)
			}
		}
	}

	if len(candidates) == 0 {
		return "", func() error { return nil }() // No available nodes
	}

	// 2. Simple score-based selection
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

	// 3. Mark workload
	b.State.Do(func(s *ClusterState) {
		s.NodeWorkloads[bestAgent]++
	})

	return bestAgent, nil
}
