package balancer

import (
	"FlakyOllama/pkg/shared/models"
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
func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte, clientIP string, allowHedging bool, priority int) (*http.Response, string, string, error) {
	// Find P90 latency for delay
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second // Default fallback
	}

	results := make(chan HedgedResult, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Load Shedding: If cluster is saturated, disable hedging to prevent retry storms
	// Note: Directly accessing pq.Len() is not ideal, but we'll keep the lock that was there,
	// or rather, use no lock if we believe Queue manages itself, but b.Mu was used.
	// Since no other granular mutex applies to Queue, we'll keep using b.Mu or none.
	// Actually, b.Mu.RLock() is fine for reading cluster-wide state.
	b.Mu.RLock()
	isSaturated := b.Queue.pq.Len() > 0
	b.Mu.RUnlock()

	// First attempt
	go b.singleAttempt(ctx, modelName, path, body, clientIP, priority, results)

	// Decision: Should we hedge?
	shouldHedge := allowHedging && !isSaturated

	if !shouldHedge {
		select {
		case res := <-results:
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				return res.Resp, res.AgentID, res.AgentAddr, nil
			}
			if res.Resp != nil {
				res.Resp.Body.Close()
			}
			if res.Err != nil {
				return nil, "", "", res.Err
			}
			return nil, "", "", fmt.Errorf("agent %s returned status %d", res.AgentID, res.Resp.StatusCode)
		case <-ctx.Done():
			return nil, "", "", ctx.Err()
		}
	}

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

			if !secondStarted {
				secondStarted = true
				timer.Stop()
				go b.singleAttempt(ctx, modelName, path, body, clientIP, priority, results)
			} else if i == 1 {
				if firstErr != nil {
					return nil, "", "", firstErr
				}
				return nil, "", "", fmt.Errorf("all attempts failed")
			}
		case <-timer.C:
			if !secondStarted {
				secondStarted = true
				go b.singleAttempt(ctx, modelName, path, body, clientIP, priority, results)
			}
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

func (b *Balancer) singleAttempt(ctx context.Context, modelName, path string, body []byte, clientIP string, priority int, results chan<- HedgedResult) {
	resCh := b.Queue.Push(models.InferenceRequest{Model: modelName}, priority, clientIP, ctx)

	var qr QueuedResponse
	select {
	case <-ctx.Done():
		// If we are canceled while in queue, Route hasn't been called, so no workload incremented
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

	scheme := "http"
	if b.Config.TLS.Enabled {
		scheme = "https"
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", scheme+"://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.recordError(addr)
		// Decrement workload since the attempt failed immediately
		b.workloadMu.Lock()
		b.NodeWorkloads[addr]--
		b.workloadMu.Unlock()

		select {
		case b.MetricCh <- metricEntry{id, modelName, 0, false}:
		default:
		}
		results <- HedgedResult{Err: err, AgentID: id, AgentAddr: addr}
		return
	}

	// Wrap body to decrement workload on close
	resp.Body = &workloadBody{ReadCloser: resp.Body, b: b, addr: addr}
	results <- HedgedResult{Resp: resp, AgentID: id, AgentAddr: addr, Err: nil}
}
