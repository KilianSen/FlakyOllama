package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"net/http"
	"time"
)

func (b *Balancer) DoHedgedRequest(ctx context.Context, model, path string, body []byte, clientIP string, allowHedging bool, priority int, contextHash string) (*http.Response, time.Duration, string, error) {
	p90, _ := b.Storage.GetP90Latency(model)
	if p90 == 0 {
		p90 = 2 * time.Second
	}

	// 1. Enter Priority Queue
	resCh := b.Queue.Push(models.InferenceRequest{Model: model}, priority, clientIP, contextHash, ctx)

	var agentAddr string
	select {
	case res := <-resCh:
		if res.Err != nil {
			return nil, p90, "", res.Err
		}
		agentAddr = res.AgentAddr
	case <-ctx.Done():
		return nil, p90, "", ctx.Err()
	}

	// 2. Initial Request
	resp, err := b.sendToAgentWithContext(ctx, agentAddr, path, body)
	if err == nil && resp.StatusCode < 400 {
		return resp, p90, agentAddr, nil
	}

	// If initial failed and hedging disabled, return
	if !allowHedging || !b.Config.EnableHedging {
		if err != nil {
			return nil, p90, agentAddr, err
		}
		return resp, p90, agentAddr, nil
	}

	// 3. Hedging Logic (simplified for now - just one retry if first failed)
	logging.Global.Debugf("Initial request to %s failed, attempting hedge", agentAddr)
	newResCh := b.Queue.Push(models.InferenceRequest{Model: model}, priority+1, clientIP, contextHash, ctx)
	select {
	case res := <-newResCh:
		if res.Err != nil {
			return nil, p90, "", res.Err
		}
		r, e := b.sendToAgentWithContext(ctx, res.AgentAddr, path, body)
		return r, p90, res.AgentAddr, e
	case <-ctx.Done():
		return nil, p90, "", ctx.Err()
	}
}
