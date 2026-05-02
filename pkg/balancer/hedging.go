package balancer

import (
	models2 "FlakyOllama/pkg/balancer/models"
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
	cancel    context.CancelFunc // cancels this request's context; nil for non-hedged
	err       error
}

func (b *Balancer) DoHedgedRequest(ctx context.Context, model, path string, body []byte, clientIP string, allowHedging bool, priority int, contextHash string) (*http.Response, time.Duration, string, error) {
	p90, _ := b.Storage.GetP90Latency(model)
	if p90 == 0 {
		p90 = 2 * time.Second
	}

	var userID string
	var isAdmin bool
	if val := ctx.Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models2.User); ok {
			userID = u.ID
			isAdmin = u.IsAdmin
		}
	}
	forceOwnNode, _ := ctx.Value(ContextKeyForceOwnNode).(bool)

	// Non-hedging fast path: use the parent context directly so that cancelling
	// the response body (after this function returns) is controlled by the caller,
	// not by a defer inside here.
	if !allowHedging || !b.Config.EnableHedging {
		resCh := b.Queue.Push(models.InferenceRequest{Model: model}, priority, clientIP, contextHash, userID, isAdmin, forceOwnNode, ctx)
		select {
		case res := <-resCh:
			if res.Err != nil {
				return nil, p90, "", res.Err
			}

			// If it was a virtual model, we MUST update the body so the agent knows which
			// actual model to use.
			finalBody := body
			if res.ResolvedModel != "" && res.ResolvedModel != model {
				finalBody = b.updateBodyModel(body, res.ResolvedModel)
			}

			resp, err := b.sendToAgentWithContext(ctx, res.AgentAddr, path, finalBody)
			if err != nil {
				b.recordError(res.AgentAddr, "agent_error")
				return nil, p90, res.AgentAddr, err
			}
			if resp.StatusCode >= 400 {
				b.recordError(res.AgentAddr, fmt.Sprintf("status_%d", resp.StatusCode))
			}
			return resp, p90, res.AgentAddr, nil
		case <-ctx.Done():
			return nil, p90, "", ctx.Err()
		}
	}

	// Hedging path: each request gets its own child context so only the loser
	// is cancelled when the winner is found.
	resultCh := make(chan hedgeResult, 2)

	sendRequest := func(p int) {
		reqCtx, cancel := context.WithCancel(ctx)
		resCh := b.Queue.Push(models.InferenceRequest{Model: model}, p, clientIP, contextHash, userID, isAdmin, forceOwnNode, reqCtx)
		select {
		case res := <-resCh:
			if res.Err != nil {
				cancel()
				resultCh <- hedgeResult{err: res.Err}
				return
			}

			finalBody := body
			if res.ResolvedModel != "" && res.ResolvedModel != model {
				finalBody = b.updateBodyModel(body, res.ResolvedModel)
			}

			resp, err := b.sendToAgentWithContext(reqCtx, res.AgentAddr, path, finalBody)
			// Pass cancel along so the caller can cancel the loser but not the winner.
			resultCh <- hedgeResult{resp: resp, agentAddr: res.AgentAddr, cancel: cancel, err: err}
		case <-reqCtx.Done():
			cancel()
			resultCh <- hedgeResult{err: reqCtx.Err()}
		}
	}

	go sendRequest(priority)

	hedgeTimer := time.AfterFunc(p90, func() {
		logging.Global.Debugf("P90 (%v) exceeded for %s, sending hedge request", p90, model)
		go sendRequest(priority + 1)
	})
	defer hedgeTimer.Stop()

	var firstErr error
	var loserCancel context.CancelFunc

	for i := 0; i < 2; i++ {
		select {
		case res := <-resultCh:
			if res.err == nil && res.resp != nil && res.resp.StatusCode < 400 {
				// Winner found — cancel the other request if it's running.
				if loserCancel != nil {
					loserCancel()
				}
				return res.resp, p90, res.agentAddr, nil
			}

			// This attempt failed; cancel its context if present.
			if res.cancel != nil {
				res.cancel()
			}

			if res.agentAddr != "" {
				if res.err != nil {
					b.recordError(res.agentAddr, "agent_error")
				} else if res.resp != nil && res.resp.StatusCode >= 400 {
					b.recordError(res.agentAddr, fmt.Sprintf("status_%d", res.resp.StatusCode))
				}
			}

			if res.err != nil && firstErr == nil {
				firstErr = res.err
			} else if res.resp != nil && res.resp.StatusCode >= 400 && firstErr == nil {
				firstErr = fmt.Errorf("agent returned status %d", res.resp.StatusCode)
			}

			// Remember the cancel of the first failed attempt; if a second attempt
			// comes back as a winner, we don't need it, but if it also fails, we're done.
			if loserCancel == nil && res.cancel != nil {
				loserCancel = res.cancel
			}

		case <-ctx.Done():
			if loserCancel != nil {
				loserCancel()
			}
			return nil, p90, "", ctx.Err()
		}
	}

	return nil, p90, "", firstErr
}
