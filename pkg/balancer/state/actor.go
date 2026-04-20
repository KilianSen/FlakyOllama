package state

import (
	"FlakyOllama/pkg/shared/models"
	"sync"
	"time"
)

// ClusterState holds the authoritative state of the cluster.
type ClusterState struct {
	Agents          map[string]*models.NodeStatus
	PendingRequests map[string]int       // model_name -> count
	InProgressPulls map[string]time.Time // model_name -> start_time
	NodeWorkloads   map[string]int       // agent_addr -> count
	ModelLastUsed   map[string]time.Time // "node_id:model_name" -> last_time
}

// StateSnapshot is a point-in-time copy of the cluster state for reading.
type StateSnapshot struct {
	Agents          map[string]models.NodeStatus
	PendingRequests map[string]int
	InProgressPulls map[string]time.Time
	NodeWorkloads   map[string]int
}

// Message types for the Actor
type Action func(*ClusterState)

type ClusterStateActor struct {
	state   *ClusterState
	actions chan Action
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func NewClusterStateActor() *ClusterStateActor {
	return &ClusterStateActor{
		state: &ClusterState{
			Agents:          make(map[string]*models.NodeStatus),
			PendingRequests: make(map[string]int),
			InProgressPulls: make(map[string]time.Time),
			NodeWorkloads:   make(map[string]int),
			ModelLastUsed:   make(map[string]time.Time),
		},
		actions: make(chan Action, 100),
		stopCh:  make(chan struct{}),
	}
}

func (a *ClusterStateActor) Start() {
	a.wg.Add(1)
	go a.run()
}

func (a *ClusterStateActor) Stop() {
	close(a.stopCh)
	a.wg.Wait()
}

func (a *ClusterStateActor) run() {
	defer a.wg.Done()
	for {
		select {
		case action := <-a.actions:
			action(a.state)
		case <-a.stopCh:
			return
		}
	}
}

// Do executes a mutation or query synchronously by waiting for the action to complete.
func (a *ClusterStateActor) Do(action Action) {
	done := make(chan struct{})
	a.actions <- func(s *ClusterState) {
		action(s)
		close(done)
	}
	<-done
}

// DoAsync executes a mutation asynchronously.
func (a *ClusterStateActor) DoAsync(action Action) {
	a.actions <- action
}

// Helper methods for common operations

func (a *ClusterStateActor) GetSnapshot() StateSnapshot {
	var snapshot StateSnapshot
	a.Do(func(s *ClusterState) {
		snapshot = StateSnapshot{
			Agents:          make(map[string]models.NodeStatus),
			PendingRequests: make(map[string]int),
			InProgressPulls: make(map[string]time.Time),
			NodeWorkloads:   make(map[string]int),
		}
		for addr, agent := range s.Agents {
			snapshot.Agents[addr] = *agent
		}
		for m, c := range s.PendingRequests {
			snapshot.PendingRequests[m] = c
		}
		for m, t := range s.InProgressPulls {
			snapshot.InProgressPulls[m] = t
		}
		for addr, c := range s.NodeWorkloads {
			snapshot.NodeWorkloads[addr] = c
		}
	})
	return snapshot
}

func (a *ClusterStateActor) UpdateNode(addr string, update func(*models.NodeStatus)) {
	a.Do(func(s *ClusterState) {
		if node, ok := s.Agents[addr]; ok {
			update(node)
		}
	})
}

func (a *ClusterStateActor) UpsertNode(addr string, node *models.NodeStatus) {
	a.DoAsync(func(s *ClusterState) {
		s.Agents[addr] = node
	})
}
