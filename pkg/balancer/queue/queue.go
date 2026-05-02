package queue

import (
	"FlakyOllama/pkg/shared/models"
	"context"
	"time"
)

// QueuedResponse represents the result of processing a queued request.
type QueuedResponse struct {
	AgentID       string
	AgentAddr     string
	ResolvedModel string
	Err           error
}

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
