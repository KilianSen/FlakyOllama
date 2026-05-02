package queue

// PriorityQueue implements heap.Interface and holds QueuedRequests.
type PriorityQueue []*QueuedRequest

// Len is part of heap.Interface. It returns the number of elements in the collection.
func (pq PriorityQueue) Len() int { return len(pq) }

// Less is part of heap.Interface. It returns true if the element with
// index i should sort before the element with index j.
func (pq PriorityQueue) Less(i, j int) bool {
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority // Higher priority first
	}
	return pq[i].Sequence < pq[j].Sequence // Tie breaker FIFO
}

// Swap is part of heap.Interface. It swaps the elements with indexes i and j.
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

// Push and Pop are part of heap.Interface.
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueuedRequest)
	item.Index = n
	*pq = append(*pq, item)
}

// Pop removes and returns the highest priority element (according to Less).
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}
