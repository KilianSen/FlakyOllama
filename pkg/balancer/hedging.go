package balancer

import (
	"FlakyOllama/pkg/models"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// HedgedResult wraps an HTTP response or an error.
type HedgedResult struct {
	Resp    *http.Response
	AgentID string
	Err     error
}

// DoHedgedRequest sends a request to one node, and if it doesn't return headers by 'delay',
// it sends a second request to another node.
func (b *Balancer) DoHedgedRequest(ctx context.Context, modelName string, path string, body []byte) (*http.Response, string, error) {
	// Find P90 latency for delay
	p90, _ := b.Storage.GetP90Latency(modelName)
	if p90 == 0 {
		p90 = 2 * time.Second // Default fallback
	}

	results := make(chan HedgedResult, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// First attempt
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.singleAttempt(ctx, modelName, path, body, results)
	}()

	// Timer for second attempt
	timer := time.NewTimer(p90)
	defer timer.Stop()

	secondStarted := false
	
	// Wait for first result or timer
	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			if res.Err == nil && res.Resp.StatusCode == http.StatusOK {
				// Success! return and cancel others.
				return res.Resp, res.AgentID, nil
			}
			// If it failed and we haven't started the second one yet, start it now.
			if !secondStarted && i == 0 {
				secondStarted = true
				wg.Add(1)
				go func() {
					defer wg.Done()
					b.singleAttempt(ctx, modelName, path, body, results)
				}()
			}
		case <-timer.C:
			if !secondStarted {
				secondStarted = true
				wg.Add(1)
				go func() {
					defer wg.Done()
					b.singleAttempt(ctx, modelName, path, body, results)
				}()
			}
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}

	return nil, "", io.EOF
}

func (b *Balancer) singleAttempt(ctx context.Context, modelName, path string, body []byte, results chan<- HedgedResult) {
	// Route
	id, addr, err := b.Route(models.InferenceRequest{Model: modelName})
	if err != nil {
		results <- HedgedResult{Err: err}
		return
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	results <- HedgedResult{Resp: resp, AgentID: id, Err: err}
}
