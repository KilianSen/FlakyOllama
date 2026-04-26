package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"sync"
	"time"
)

type Actor struct {
	state ClusterState
	mu    sync.RWMutex
}

type ClusterState struct {
	Agents          map[string]*models.NodeStatus
	ActiveWorkloads int
	AvgCPUUsage     float64
	AvgMemUsage     float64
	PendingRequests map[string]int
	ModelLastUsed   map[string]time.Time
	NodeWorkloads   map[string]int
}

func NewActor() *Actor {
	return &Actor{
		state: ClusterState{
			Agents:          make(map[string]*models.NodeStatus),
			PendingRequests: make(map[string]int),
			ModelLastUsed:   make(map[string]time.Time),
			NodeWorkloads:   make(map[string]int),
		},
	}
}

func (a *Actor) Do(f func(*ClusterState)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f(&a.state)
}

func (a *Actor) View(f func(ClusterState)) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	f(a.state)
}

func (a *Actor) GetSnapshot() ClusterState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Deep copy maps
	snap := ClusterState{
		Agents:          make(map[string]*models.NodeStatus),
		ActiveWorkloads: a.state.ActiveWorkloads,
		AvgCPUUsage:     a.state.AvgCPUUsage,
		AvgMemUsage:     a.state.AvgMemUsage,
		PendingRequests: make(map[string]int),
		ModelLastUsed:   make(map[string]time.Time),
		NodeWorkloads:   make(map[string]int),
	}

	for k, v := range a.state.Agents {
		nodeCopy := *v
		// Deep copy slices
		if v.ActiveModels != nil {
			nodeCopy.ActiveModels = make([]string, len(v.ActiveModels))
			copy(nodeCopy.ActiveModels, v.ActiveModels)
		}
		if v.LocalModels != nil {
			nodeCopy.LocalModels = make([]models.ModelInfo, len(v.LocalModels))
			copy(nodeCopy.LocalModels, v.LocalModels)
		}
		snap.Agents[k] = &nodeCopy
	}
	for k, v := range a.state.PendingRequests {
		snap.PendingRequests[k] = v
	}
	for k, v := range a.state.ModelLastUsed {
		snap.ModelLastUsed[k] = v
	}
	for k, v := range a.state.NodeWorkloads {
		snap.NodeWorkloads[k] = v
	}

	return snap
}
