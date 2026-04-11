package balancer

import (
	"FlakyOllama/pkg/models"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// HedgedResult wraps an HTTP response or an error.
type HedgedResult struct {
	Resp      *http.Response
	AgentID   string
	AgentAddr string
	Err       error
}

// DoHedgedRequest sends a request to one node, and if it doesn't return headers by 'delay',
// it sends a second request to another node.
func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte, clientIP string) (*http.Response, string, string, error) {
	// Find P90 latency for delay
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second // Default fallback
	}

	results := make(chan HedgedResult, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Load Shedding: If cluster is saturated, disable hedging to prevent retry storms
	b.Mu.RLock()
	isSaturated := b.Queue.pq.Len() > 0
	b.Mu.RUnlock()

	// First attempt
	go b.singleAttempt(ctx, modelName, path, body, clientIP, results)

	if isSaturated {
		// Just wait for the first attempt or timeout
		select {
		case res := <-results:
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				return res.Resp, res.AgentID, res.AgentAddr, nil
			}
			if res.Err != nil {
				return nil, "", "", res.Err
			}
			return nil, "", "", fmt.Errorf("agent returned status %d", res.Resp.StatusCode)
		case <-ctx.Done():
			return nil, "", "", ctx.Err()
		}
	}

	// Timer for second attempt
	timer := time.NewTimer(p90)
	defer timer.Stop()

	var firstErr error
	var secondStarted bool

	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				return res.Resp, res.AgentID, res.AgentAddr, nil
			}

			if res.Err != nil {
				firstErr = res.Err
			} else if res.Resp != nil {
				firstErr = fmt.Errorf("agent %s returned status %d", res.AgentID, res.Resp.StatusCode)
				res.Resp.Body.Close()
			}

			// If this was the first result and it failed, and we haven't started the second one, start it now.
			if !secondStarted {
				secondStarted = true
				timer.Stop() // No need to wait for timer anymore
				go b.singleAttempt(ctx, modelName, path, body, clientIP, results)
			} else if i == 1 {
				// This was the second result (either second attempt finished, or first finished after second started)
				// and it also failed.
				if firstErr != nil {
					return nil, "", "", firstErr
				}
				return nil, "", "", fmt.Errorf("all attempts failed")
			}
		case <-timer.C:
			if !secondStarted {
				secondStarted = true
				go b.singleAttempt(ctx, modelName, path, body, clientIP, results)
			}
			// Don't increment i here, we still want to wait for 2 results if possible
			i--
		case <-ctx.Done():
			return nil, "", "", ctx.Err()
		}
	}

	if firstErr != nil {
		return nil, "", "", firstErr
	}
	return nil, "", "", io.EOF
}

func (b *Balancer) singleAttempt(ctx context.Context, modelName, path string, body []byte, clientIP string, results chan<- HedgedResult) {
	// Route via PriorityQueue to ensure proper load management
	resCh := b.Queue.Push(models.InferenceRequest{Model: modelName}, 0, clientIP, ctx)

	var qr QueuedResponse
	select {
	case <-ctx.Done():
		results <- HedgedResult{Err: ctx.Err()}
		return
	case qr = <-resCh:
		if qr.Err != nil {
			results <- HedgedResult{Err: qr.Err}
			return
		}
	}

	id := qr.AgentID
	addr := qr.AgentAddr

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.recordError(addr)
		select {
		case b.MetricCh <- metricEntry{id, modelName, 0, false}:
		default:
		}
	}
	results <- HedgedResult{Resp: resp, AgentID: id, AgentAddr: addr, Err: err}
}
