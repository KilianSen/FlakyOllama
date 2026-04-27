package balancer

import (
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"fmt"
	"net/http"
	"time"
)

type hedgeResult struct {
	resp      *http.Response
	p90       time.Duration
	agentAddr string
	err       error
}

func (b *Balancer) DoHedgedRequest(ctx context.Context, model, path string, body []byte, clientIP string, allowHedging bool, priority int, contextHash string) (*http.Response, time.Duration, string, error) {
	p90, _ := b.Storage.GetP90Latency(model)
	if p90 == 0 {
		p90 = 2 * time.Second
	}

	var userID string
	if val := ctx.Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok {
			userID = u.ID
		}
	}

	resultCh := make(chan hedgeResult, 2)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sendRequest := func(p int) {
		resCh := b.Queue.Push(models.InferenceRequest{Model: model}, p, clientIP, contextHash, userID, ctx)
		select {
		case res := <-resCh:
			if res.Err != nil {
				resultCh <- hedgeResult{err: res.Err}
				return
			}
			resp, err := b.sendToAgentWithContext(ctx, res.AgentAddr, path, body)
			resultCh <- hedgeResult{resp: resp, agentAddr: res.AgentAddr, err: err}
		case <-ctx.Done():
			resultCh <- hedgeResult{err: ctx.Err()}
		}
	}

	// 1. Initial Request
	go sendRequest(priority)

	// 2. Hedge Request (if enabled)
	var hedgeTimer *time.Timer
	if allowHedging && b.Config.EnableHedging {
		hedgeTimer = time.AfterFunc(p90, func() {
			logging.Global.Debugf("P90 (%v) exceeded for %s, sending hedge request", p90, model)
			sendRequest(priority + 1)
		})
		defer hedgeTimer.Stop()
	}

	var firstErr error
	for i := 0; i < 2; i++ {
		select {
		case res := <-resultCh:
			if res.err == nil && res.resp != nil && res.resp.StatusCode < 400 {
				cancel() // Stop the other request
				return res.resp, p90, res.agentAddr, nil
			}
			if res.err != nil && firstErr == nil {
				firstErr = res.err
			} else if res.resp != nil && res.resp.StatusCode >= 400 && firstErr == nil {
				firstErr = fmt.Errorf("agent returned status %d", res.resp.StatusCode)
			}

			if !allowHedging || !b.Config.EnableHedging {
				return res.resp, p90, res.agentAddr, res.err
			}
		case <-ctx.Done():
			return nil, p90, "", ctx.Err()
		}
	}

	return nil, p90, "", firstErr
}
