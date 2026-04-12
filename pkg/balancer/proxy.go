package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/metrics"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
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
		w.b.workloadMu.Lock()
		w.b.NodeWorkloads[w.addr]--
		w.b.workloadMu.Unlock()
	})
	return err
}

func (b *Balancer) sendToAgent(addr, path string, body []byte) (*http.Response, error) {
	scheme := "http"
	if b.Config.TLS.Enabled {
		scheme = "https"
	}
	req, _ := http.NewRequest("POST", scheme+"://"+addr+path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("AGENT_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
		b.recordError(agentAddr)
		select {
		case b.MetricCh <- metricEntry{agentAddr, modelName, latency, false}:
		default:
		}
		if errors.Is(err, ErrStalled) {
			logging.Global.Warnf("Agent %s stalled during stream for model %s", agentAddr, modelName)
		} else {
			logging.Global.Errorf("Stream error from %s: %v", agentAddr, err)
		}
		return
	}

	b.recordSuccess(agentAddr)
	select {
	case b.MetricCh <- metricEntry{agentAddr, modelName, latency, true}:
	default:
	}
	// We still use ID for some metrics if we want to aggregate by "name",
	// but here we should probably use Address or ID:Address
	agentID := agentAddr
	b.Mu.RLock()
	if a, ok := b.Agents[agentAddr]; ok {
		agentID = a.ID
	}
	b.Mu.RUnlock()

	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(latency.Seconds())

	b.lastUsedMu.Lock()
	b.ModelLastUsed[agentAddr+":"+modelName] = time.Now()
	b.lastUsedMu.Unlock()
}

func (b *Balancer) recordError(addr string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[addr]; ok {
		a.Errors++
		oldState := a.State
		if a.Errors >= b.Config.CircuitBreaker.ErrorThreshold {
			a.State = models.StateBroken
			a.CooloffUntil = time.Now().Add(time.Duration(b.Config.CircuitBreaker.CooloffSec) * time.Second)
		} else {
			a.State = models.StateDegraded
		}
		if oldState != a.State {
			logging.Global.Infof("Node %s (%s) state changed: %s -> %s (errors: %d, cooloff until: %v)", a.ID, addr, oldState.String(), a.State.String(), a.Errors, a.CooloffUntil)
		}
	}
}

func (b *Balancer) recordSuccess(addr string) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	if a, ok := b.Agents[addr]; ok {
		if a.State != models.StateHealthy {
			logging.Global.Infof("Node %s (%s) recovered to Healthy state", a.ID, addr)
		}
		a.Errors = 0
		a.State = models.StateHealthy
	}
}
