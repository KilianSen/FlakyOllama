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
	AgentAddr string
	Err       error
}

// DoHedgedRequest sends a request to one node, and if it doesn't return headers by 'delay',
// it sends a second request to another node.
func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte) (*http.Response, string, error) {
	// Find P90 latency for delay
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second // Default fallback
	}

	b.Mu.RLock()
	hedgingEnabled := b.Config.EnableHedging
	b.Mu.RUnlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if !hedgingEnabled {
		results := make(chan HedgedResult, 1)
		b.singleAttempt(ctx, modelName, path, body, results)
		res := <-results
		return res.Resp, res.AgentAddr, res.Err
	}

	results := make(chan HedgedResult, 2)
	attempts := 1
	go b.singleAttempt(ctx, modelName, path, body, results)

	timer := time.NewTimer(p90)
	defer timer.Stop()

	var firstErr error
	var secondStarted bool
	readCount := 0

	defer func() {
		left := attempts - readCount
		if left > 0 {
			go func() {
				for j := 0; j < left; j++ {
					res := <-results
					if res.Resp != nil {
						io.Copy(io.Discard, res.Resp.Body)
						res.Resp.Body.Close()
					}
				}
			}()
		}
	}()

	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			readCount++
			if res.Err == nil && res.Resp != nil && res.Resp.StatusCode == http.StatusOK {
				return res.Resp, res.AgentAddr, nil
			}

			if res.Err != nil {
				firstErr = res.Err
			} else if res.Resp != nil {
				firstErr = fmt.Errorf("agent %s returned status %d", res.AgentAddr, res.Resp.StatusCode)
				io.Copy(io.Discard, res.Resp.Body)
				res.Resp.Body.Close()
			}

			// If this was the first result and it failed, and we haven't started the second one, start it now.
			if !secondStarted {
				secondStarted = true
				attempts++
				timer.Stop() // No need to wait for timer anymore
				go b.singleAttempt(ctx, modelName, path, body, results)
			} else if readCount == attempts {
				if firstErr != nil {
					return nil, "", firstErr
				}
				return nil, "", fmt.Errorf("all attempts failed")
			}
		case <-timer.C:
			if !secondStarted {
				secondStarted = true
				attempts++
				go b.singleAttempt(ctx, modelName, path, body, results)
			}
			// Don't increment i here, we still want to wait for 2 results if possible
			i--
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}

	if firstErr != nil {
		return nil, "", firstErr
	}
	return nil, "", io.EOF
}

func (b *Balancer) singleAttempt(ctx context.Context, modelName, path string, body []byte, results chan<- HedgedResult) {
	// Route
	_, addr, err := b.Route(models.InferenceRequest{Model: modelName})
	if err != nil {
		results <- HedgedResult{Err: err}
		return
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		b.recordError(addr)
		b.Storage.RecordMetric(addr, modelName, 0, false)
	}
	results <- HedgedResult{Resp: resp, AgentAddr: addr, Err: err}
}
