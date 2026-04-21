package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HedgedResult wraps an HTTP response or an error.
type HedgedResult struct {
	Resp      *http.Response
	AgentID   string
	AgentAddr string
	Err       error
	Cancel    context.CancelFunc // Function to cancel this specific attempt's context
}

// DoHedgedRequest sends a request to one node, and if it doesn't return headers by 'delay',
// it sends a second request to another node.
func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte, clientIP string, allowHedging bool, priority int) (*http.Response, string, string, error) {
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second
	}

	results := make(chan HedgedResult, 2)

	// Track which attempt won to avoid canceling its context prematurely
	var winningAddr string

	// Cleanup goroutine to ensure no response bodies are leaked
	defer func() {
		go func() {
			for i := 0; i < 2; i++ {
				select {
				case res := <-results:
					if res.AgentAddr != winningAddr && res.Resp != nil {
						res.Resp.Body.Close()
					}
					if res.AgentAddr != winningAddr && res.Cancel != nil {
						res.Cancel()
					}
				case <-time.After(5 * time.Minute):
					return
				}
			}
		}()
	}()

	isSaturated := b.Queue.pq.Len() > 0

	// First attempt
	ctx1, cancel1 := context.WithCancel(ctx)
	go b.singleAttempt(ctx1, cancel1, modelName, path, body, clientIP, priority, results)

	shouldHedge := allowHedging && !isSaturated

	if !shouldHedge {
		select {
		case res := <-results:
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				winningAddr = res.AgentAddr
				return res.Resp, res.AgentID, res.AgentAddr, nil
			}
			if res.Resp != nil {
				res.Resp.Body.Close()
			}
			if res.Cancel != nil {
				res.Cancel()
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
				winningAddr = res.AgentAddr
				return res.Resp, res.AgentID, res.AgentAddr, nil
			}

			if res.Resp != nil {
				res.Resp.Body.Close()
			}
			if res.Cancel != nil {
				res.Cancel()
			}
			if res.Err != nil {
				firstErr = res.Err
			} else if res.Resp != nil {
				firstErr = fmt.Errorf("agent %s returned status %d", res.AgentID, res.Resp.StatusCode)
			}

			if !secondStarted {
				secondStarted = true
				timer.Stop()
				ctx2, cancel2 := context.WithCancel(ctx)
				go b.singleAttempt(ctx2, cancel2, modelName, path, body, clientIP, priority, results)
			} else if i == 1 {
				if firstErr != nil {
					return nil, "", "", firstErr
				}
				return nil, "", "", fmt.Errorf("all attempts failed")
			}
		case <-timer.C:
			if !secondStarted {
				secondStarted = true
				ctx2, cancel2 := context.WithCancel(ctx)
				go b.singleAttempt(ctx2, cancel2, modelName, path, body, clientIP, priority, results)
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

func (b *Balancer) singleAttempt(ctx context.Context, cancel context.CancelFunc, modelName, path string, body []byte, clientIP string, priority int, results chan<- HedgedResult) {
	resCh := b.Queue.Push(models.InferenceRequest{Model: modelName}, priority, clientIP, ctx)

	var qr QueuedResponse
	select {
	case <-ctx.Done():
		results <- HedgedResult{Err: ctx.Err(), Cancel: cancel}
		return
	case qr = <-resCh:
		if qr.Err != nil {
			results <- HedgedResult{Err: qr.Err, Cancel: cancel}
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
	if b.Config.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.Config.RemoteToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.recordError(addr, "http_error")
		b.State.DoAsync(func(s *state.ClusterState) {
			s.NodeWorkloads[addr]--
		})

		select {
		case b.MetricCh <- metricEntry{id, modelName, 0, false}:
		default:
		}
		results <- HedgedResult{Err: err, AgentID: id, AgentAddr: addr, Cancel: cancel}
		return
	}

	// Wrap body to decrement workload on close.
	wrapped := &workloadBody{ReadCloser: resp.Body, b: b, addr: addr}
	resp.Body = &cancelBody{ReadCloser: wrapped, cancel: cancel}

	results <- HedgedResult{Resp: resp, AgentID: id, AgentAddr: addr, Err: nil, Cancel: cancel}
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelBody) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(c.cancel)
	return err
}
