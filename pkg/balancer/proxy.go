package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/metrics"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

type workloadBody struct {
	io.ReadCloser
	b    *Balancer
	addr string
	once sync.Once
}

func (w *workloadBody) Close() error {
	err := w.ReadCloser.Close()
	w.once.Do(func() {
		w.b.State.Do(func(s *state.ClusterState) {
			s.NodeWorkloads[w.addr]--
		})
	})
	return err
}

func (b *Balancer) sendToAgent(addr, path string, body []byte) (*http.Response, error) {
	return b.sendToAgentWithContext(context.Background(), addr, path, body)
}

func (b *Balancer) sendToAgentWithContext(ctx context.Context, addr, path string, body []byte) (*http.Response, error) {
	scheme := "http"
	if b.Config.TLS.Enabled {
		scheme = "https"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", scheme+"://"+addr+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.Config.RemoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.Config.RemoteToken)
	}
	return b.httpClient.Do(req)
}

func (b *Balancer) finalizeProxy(w http.ResponseWriter, resp *http.Response, agentAddr, modelName string) {
	start := time.Now()
	// Wrap with Stall Protection
	stallTimeout := time.Duration(b.Config.StallTimeoutSec) * time.Second
	reader := NewIdleTimeoutReader(resp.Body, stallTimeout)
	defer reader.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	_, err := io.Copy(w, reader)
	latency := time.Since(start)
	if err != nil {
		reason := "stream_error"
		if errors.Is(err, ErrStalled) {
			reason = "stalled"
			logging.Global.Warnf("Agent %s stalled during stream for model %s", agentAddr, modelName)
		} else {
			logging.Global.Errorf("Stream error from %s: %v", agentAddr, err)
		}
		b.recordError(agentAddr, reason)
		select {
		case b.MetricCh <- metricEntry{agentAddr, modelName, latency, false}:
		default:
		}
		return
	}

	b.recordSuccess(agentAddr)
	select {
	case b.MetricCh <- metricEntry{agentAddr, modelName, latency, true}:
	default:
	}

	agentID := agentAddr
	b.State.Do(func(s *state.ClusterState) {
		if a, ok := s.Agents[agentAddr]; ok {
			agentID = a.ID
		}
		s.ModelLastUsed[agentAddr+":"+modelName] = time.Now()
	})

	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(latency.Seconds())
}

func (b *Balancer) recordError(addr string, reason string) {
	b.State.Do(func(s *state.ClusterState) {
		if a, ok := s.Agents[addr]; ok {
			a.Errors++
			a.Message = "Error: " + reason
			oldState := a.State
			if a.Errors >= b.Config.CircuitBreaker.ErrorThreshold {
				a.State = models.StateBroken
				a.CooloffUntil = time.Now().Add(time.Duration(b.Config.CircuitBreaker.CooloffSec) * time.Second)
				a.Message = "Broken: too many errors (" + reason + ")"
			} else {
				a.State = models.StateDegraded
			}
			if oldState != a.State {
				logging.Global.Infof("Node %s (%s) state changed: %s -> %s (reason: %s, errors: %d, cooloff until: %v)",
					a.ID, addr, oldState.String(), a.State.String(), reason, a.Errors, a.CooloffUntil)
			}
		}
	})
}

func (b *Balancer) recordSuccess(addr string) {
	b.State.Do(func(s *state.ClusterState) {
		if a, ok := s.Agents[addr]; ok {
			if a.State != models.StateHealthy {
				logging.Global.Infof("Node %s (%s) recovered to Healthy state", a.ID, addr)
			}
			a.Errors = 0
			a.State = models.StateHealthy
			a.Message = "Ready"
		}
	})
}
