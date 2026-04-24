package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HedgedResult wraps an HTTP response or an error.
type HedgedResult struct {
	Resp      *http.Response
	AgentID   string
	AgentAddr string
	Err       error
	Cancel    context.CancelFunc
}

// peekReader signals when the first byte is read
type peekReader struct {
	io.ReadCloser
	onFirstByte func()
	once        sync.Once
}

func (p *peekReader) Read(buf []byte) (int, error) {
	n, err := p.ReadCloser.Read(buf)
	if n > 0 {
		p.once.Do(p.onFirstByte)
	}
	return n, err
}

func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte, clientIP string, allowHedging bool, priority int, contextHash string) (*http.Response, string, string, error) {
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second
	}

	results := make(chan HedgedResult, 2)
	var winningAddr string
	var mu sync.Mutex

	// First attempt
	ctx1, cancel1 := context.WithCancel(ctx)
	go b.singleAttemptSpeculative(ctx1, cancel1, modelName, path, body, clientIP, priority, contextHash, results)

	// Check if this is a targeted request (Playground node selection)
	isTargeted := strings.HasPrefix(contextHash, "node-") || (len(contextHash) > 0 && !strings.Contains(contextHash, " ")) // basic heuristic

	// Override shouldHedge if targeted
	if isTargeted {
		allowHedging = false
	}

	shouldHedge := allowHedging && b.Queue.pq.Len() == 0

	timer := time.NewTimer(p90)
	defer timer.Stop()

	var firstErr error
	var secondStarted bool

	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				mu.Lock()
				if winningAddr == "" {
					winningAddr = res.AgentAddr
					mu.Unlock()

					// Record Reputation Change: Winner
					go func(addr string, data chan HedgedResult) {
						// Record win
						b.State.Do(func(s *state.ClusterState) {
							if a, ok := s.Agents[addr]; ok && a.AgentKey != "" {
								b.Storage.RecordReputation(a.AgentKey, 0.01)
							}
						})

						// Record loss for the other if it eventually finishes
						if secondStarted {
							select {
							case other := <-data:
								if other.AgentAddr != addr && other.AgentAddr != "" {
									b.State.Do(func(s *state.ClusterState) {
										if a, ok := s.Agents[other.AgentAddr]; ok && a.AgentKey != "" {
											b.Storage.RecordReputation(a.AgentKey, -0.05)
										}
									})
								}
							case <-time.After(10 * time.Second):
							}
						}
					}(res.AgentAddr, results)

					return res.Resp, res.AgentID, res.AgentAddr, nil
				}
				mu.Unlock()

				res.Resp.Body.Close()
				if res.Cancel != nil {
					res.Cancel()
				}
				continue
			}

			if res.Resp != nil {
				res.Resp.Body.Close()
			}
			if res.Cancel != nil {
				res.Cancel()
			}

			if res.Err != nil {
				firstErr = res.Err
			}

			if !secondStarted && shouldHedge {
				secondStarted = true
				timer.Stop()
				ctx2, cancel2 := context.WithCancel(ctx)
				go b.singleAttemptSpeculative(ctx2, cancel2, modelName, path, body, clientIP, priority, contextHash, results)
			} else if i == 1 || !shouldHedge {
				return nil, "", "", firstErr
			}
		case <-timer.C:
			if !secondStarted && shouldHedge {
				secondStarted = true
				ctx2, cancel2 := context.WithCancel(ctx)
				go b.singleAttemptSpeculative(ctx2, cancel2, modelName, path, body, clientIP, priority, contextHash, results)
			}
			i--
		case <-ctx.Done():
			return nil, "", "", ctx.Err()
		}
	}

	return nil, "", "", firstErr
}

func (b *Balancer) singleAttemptSpeculative(ctx context.Context, cancel context.CancelFunc, modelName, path string, body []byte, clientIP string, priority int, contextHash string, results chan<- HedgedResult) {
	resCh := b.Queue.Push(models.InferenceRequest{Model: modelName}, priority, clientIP, contextHash, ctx)

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

	scheme := "http"
	if b.Config.TLS.Enabled {
		scheme = "https"
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", scheme+"://"+qr.AgentAddr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if b.Config.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.Config.RemoteToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.recordError(qr.AgentAddr, "http_error")
		// Decrement workload since we didn't start the token race
		b.State.DoAsync(func(s *state.ClusterState) { s.NodeWorkloads[qr.AgentAddr]-- })
		results <- HedgedResult{Err: err, AgentID: qr.AgentID, AgentAddr: qr.AgentAddr, Cancel: cancel}
		return
	}

	if resp.StatusCode != http.StatusOK {
		// Decrement workload for non-200 responses
		b.State.DoAsync(func(s *state.ClusterState) { s.NodeWorkloads[qr.AgentAddr]-- })
		results <- HedgedResult{Resp: resp, AgentID: qr.AgentID, AgentAddr: qr.AgentAddr, Cancel: cancel}
		return
	}

	// The "Speculative" part: wait for the first byte before considering this attempt a winner
	firstByteReceived := make(chan struct{})
	peeker := &peekReader{
		ReadCloser:  resp.Body,
		onFirstByte: func() { close(firstByteReceived) },
	}

	// Replace body with peeker
	wrapped := &workloadBody{ReadCloser: peeker, b: b, addr: qr.AgentAddr}
	resp.Body = &cancelBody{ReadCloser: wrapped, cancel: cancel}

	// In speculative mode, we block here until first byte OR context cancel
	// This ensures that the "winner" is actually generating tokens.
	go func() {
		select {
		case <-firstByteReceived:
			results <- HedgedResult{Resp: resp, AgentID: qr.AgentID, AgentAddr: qr.AgentAddr, Err: nil, Cancel: cancel}
		case <-ctx.Done():
			resp.Body.Close()
		}
	}()
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
