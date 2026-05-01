package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"
)

// QueuedRequest represents a request waiting in the queue.
type QueuedRequest struct {
	ID           string
	Request      models.InferenceRequest
	Priority     int // Higher value means higher priority
	Sequence     int64
	ClientIP     string
	ContextHash  string
	UserID       string
	IsAdmin      bool
	ForceOwnNode bool // When true, SelectAgent must route only to nodes owned by UserID
	Ctx          context.Context
	QueuedAt     time.Time
	Response     chan QueuedResponse
	Index        int // The index of the item in the heap.
}

type QueuedResponse struct {
	AgentID       string
	AgentAddr     string
	ResolvedModel string
	Err           error
}

// PriorityQueue implements heap.Interface and holds QueuedRequests.
type PriorityQueue []*QueuedRequest

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	return pq[i].Sequence < pq[j].Sequence
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueuedRequest)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}

// RequestQueue handles thread-safe priority queuing.
type RequestQueue struct {
	pq       PriorityQueue
	sequence int64
	mu       sync.Mutex
	ch       chan struct{} // signaled when a new item is pushed
}

func NewRequestQueue() *RequestQueue {
	rq := &RequestQueue{
		pq: make(PriorityQueue, 0),
		ch: make(chan struct{}, 1),
	}
	heap.Init(&rq.pq)
	return rq
}

func (rq *RequestQueue) Push(req models.InferenceRequest, priority int, clientIP, contextHash, userID string, isAdmin, forceOwnNode bool, ctx context.Context) chan QueuedResponse {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	resCh := make(chan QueuedResponse, 1)
	rq.sequence++
	item := &QueuedRequest{
		ID:           fmt.Sprintf("req_%d_%d", time.Now().Unix(), rq.sequence),
		Request:      req,
		Priority:     priority,
		Sequence:     rq.sequence,
		ClientIP:     clientIP,
		ContextHash:  contextHash,
		UserID:       userID,
		IsAdmin:      isAdmin,
		ForceOwnNode: forceOwnNode,
		Ctx:          ctx,
		QueuedAt:     time.Now(),
		Response:     resCh,
	}
	heap.Push(&rq.pq, item)

	// Signal that an item is available
	select {
	case rq.ch <- struct{}{}:
	default:
	}

	return resCh
}

// Requeue put back an existing request item
func (rq *RequestQueue) Requeue(item *QueuedRequest) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// Push it back as is (preserving original response channel and ID)
	heap.Push(&rq.pq, item)

	select {
	case rq.ch <- struct{}{}:
	default:
	}
}

func (rq *RequestQueue) Pop() *QueuedRequest {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for rq.pq.Len() > 0 {
		item := heap.Pop(&rq.pq).(*QueuedRequest)
		if item.Ctx != nil && item.Ctx.Err() != nil {
			continue
		}
		return item
	}
	return nil
}

func (rq *RequestQueue) GetSnapshot() []models.QueuedRequestInfo {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	var snapshot []models.QueuedRequestInfo
	for _, item := range rq.pq {
		snapshot = append(snapshot, models.QueuedRequestInfo{
			ID:          item.ID,
			Model:       item.Request.Model,
			Priority:    item.Priority,
			ClientIP:    item.ClientIP,
			ContextHash: item.ContextHash,
			QueuedAt:    item.QueuedAt,
		})
	}
	return snapshot
}

func (rq *RequestQueue) CancelRequest(id string) bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for i, item := range rq.pq {
		if item.ID == id {
			heap.Remove(&rq.pq, i)
			// Non-blocking send
			select {
			case item.Response <- QueuedResponse{Err: fmt.Errorf("request cancelled by administrator")}:
			default:
			}
			return true
		}
	}
	return false
}

func (rq *RequestQueue) Wait() <-chan struct{} {
	return rq.ch
}

func (rq *RequestQueue) QueueDepth() int {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	return rq.pq.Len()
}

func (rq *RequestQueue) Close() {
	close(rq.ch)
}
