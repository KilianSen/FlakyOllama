package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/auth"
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/metrics"
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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

type ttftTrackingReader struct {
	io.Reader
	onFirstByte func()
	once        sync.Once
}

func (t *ttftTrackingReader) Read(p []byte) (int, error) {
	n, err := t.Reader.Read(p)
	if n > 0 {
		t.once.Do(t.onFirstByte)
	}
	return n, err
}

func (b *Balancer) finalizeProxy(w http.ResponseWriter, resp *http.Response, agentAddr, modelName string, r *http.Request, surge float64) {
	start := time.Now()
	// Get client key from context
	clientKey, _ := auth.GetTokenFromContext(r.Context())

	// Wrap with Stall Protection
	stallTimeout := time.Duration(b.Config.StallTimeoutSec) * time.Second
	reader := NewIdleTimeoutReader(resp.Body, stallTimeout)
	defer reader.Close()

	// Instrument for TTFT
	var ttft time.Duration
	var ttftMu sync.Mutex
	ttftRecorded := false

	trackingReader := &ttftTrackingReader{
		Reader: reader,
		onFirstByte: func() {
			ttftMu.Lock()
			if !ttftRecorded {
				ttft = time.Since(start)
				ttftRecorded = true
			}
			ttftMu.Unlock()
		},
	}

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	// Use a pipe to capture the response for usage parsing if it's not a stream error
	var usageBuf bytes.Buffer
	multiWriter := io.MultiWriter(w, &usageBuf)

	var finalReader io.Reader = trackingReader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(trackingReader)
		if err == nil {
			defer gz.Close()
			finalReader = gz
		}
	}

	_, err := io.Copy(multiWriter, finalReader)
	duration := time.Since(start)

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
		case b.MetricCh <- metricEntry{agentAddr, modelName, duration, false}:
		default:
		}
		return
	}

	// Extract usage from buffer
	body := usageBuf.Bytes()
	input, output := 0, 0
	var usage struct {
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := json.Unmarshal(body, &usage); err == nil && (usage.PromptEvalCount > 0 || usage.EvalCount > 0) {
		input = usage.PromptEvalCount
		output = usage.EvalCount
	} else {
		// Streaming case
		lastOpenBrace := -1
		for i := len(body) - 1; i >= 0; i-- {
			if body[i] == '{' {
				lastOpenBrace = i
				var streamUsage struct {
					PromptEvalCount int `json:"prompt_eval_count"`
					EvalCount       int `json:"eval_count"`
				}
				if err := json.Unmarshal(body[lastOpenBrace:], &streamUsage); err == nil {
					if streamUsage.PromptEvalCount > 0 || streamUsage.EvalCount > 0 {
						input = streamUsage.PromptEvalCount
						output = streamUsage.EvalCount
						break
					}
				}
				if len(body)-i > 2048 {
					break
				}
			}
		}
	}

	agentID := agentAddr
	b.State.Do(func(s *state.ClusterState) {
		if a, ok := s.Agents[agentAddr]; ok {
			agentID = a.ID
		}
		s.ModelLastUsed[agentAddr+":"+modelName] = time.Now()
	})

	if input > 0 || output > 0 {
		ttftMu.Lock()
		actualTTFT := ttft
		ttftMu.Unlock()
		go b.captureUsage(agentID, modelName, input, output, actualTTFT, duration, clientKey, surge)
	}

	b.recordSuccess(agentAddr)
	select {
	case b.MetricCh <- metricEntry{agentAddr, modelName, duration, true}:
	default:
	}

	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(duration.Seconds())
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
