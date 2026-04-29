package balancer

import (
	"FlakyOllama/pkg/balancer/protocols"
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
		w.b.State.Do(func(s *ClusterState) {
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

	logging.Global.Infof("Forwarding request to agent %s at path %s with %s", addr, path, scheme)

	req, err := http.NewRequestWithContext(ctx, "POST", scheme+"://"+addr+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var balancerToken string
	b.State.View(func(s ClusterState) {
		if agent, ok := s.Agents[addr]; ok {
			balancerToken = agent.BalancerToken
		}
	})
	if balancerToken == "" {
		balancerToken = b.Config.RemoteToken
	}
	if balancerToken != "" {
		req.Header.Set("Authorization", "Bearer "+balancerToken)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Wrap body to track workload completion
	resp.Body = &workloadBody{
		ReadCloser: resp.Body,
		b:          b,
		addr:       addr,
	}
	return resp, nil
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

type TailReader struct {
	buffer []byte
	size   int
	head   int
}

func NewTailReader(limit int) *TailReader {
	return &TailReader{
		buffer: make([]byte, limit),
		size:   0,
		head:   0,
	}
}

func (t *TailReader) Write(p []byte) (n int, err error) {
	for _, b := range p {
		t.buffer[t.head] = b
		t.head = (t.head + 1) % len(t.buffer)
		if t.size < len(t.buffer) {
			t.size++
		}
	}
	return len(p), nil
}

func (t *TailReader) Bytes() []byte {
	res := make([]byte, t.size)
	if t.size < len(t.buffer) {
		copy(res, t.buffer[:t.size])
	} else {
		copy(res, t.buffer[t.head:])
		copy(res[len(t.buffer)-t.head:], t.buffer[:t.head])
	}
	return res
}

func (b *Balancer) finalizeProxy(w http.ResponseWriter, resp *http.Response, agentAddr, modelName string, r *http.Request, surge float64) {
	b.finalizeProxyWithAdapter(w, resp, agentAddr, modelName, r, surge, nil)
}

func (b *Balancer) finalizeProxyWithAdapter(w http.ResponseWriter, resp *http.Response, agentAddr, modelName string, r *http.Request, surge float64, adapter protocols.Adapter) {
	start := time.Now()

	clientKey, _ := auth.GetTokenFromContext(r.Context())
	var userID string
	if val := r.Context().Value(auth.ContextKeyUser); val != nil {
		if u, ok := val.(models.User); ok {
			userID = u.ID
		}
	}

	stallTimeout := time.Duration(b.Config.StallTimeoutSec) * time.Second
	reader := NewIdleTimeoutReader(resp.Body, stallTimeout)
	defer reader.Close()

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

	var finalReader io.Reader = trackingReader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(trackingReader)
		if err == nil {
			defer gz.Close()
			finalReader = gz
		}
	}

	var input, output int
	var err error

	if adapter != nil {
		// Adapter handles writing to w and translating the stream
		input, output, err = adapter.TranslateResponse(w, finalReader)
	} else {
		// Standard proxy path
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		usageTail := NewTailReader(4096)
		multiWriter := io.MultiWriter(w, usageTail)

		_, err = io.Copy(multiWriter, finalReader)
		if err == nil {
			input, output = extractUsage(usageTail.Bytes())
		}
	}

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

	agentID := agentAddr
	var providerKey string

	b.State.Do(func(s *ClusterState) {
		if a, ok := s.Agents[agentAddr]; ok {
			agentID = a.ID
			providerKey = a.AgentKey
		}
		s.ModelLastUsed[agentAddr+":"+modelName] = time.Now()
	})

	if input > 0 || output > 0 {
		ttftMu.Lock()
		actualTTFT := ttft
		ttftMu.Unlock()
		go b.captureUsage(providerKey, modelName, input, output, actualTTFT, duration, clientKey, userID, surge)
	}

	b.recordSuccess(agentAddr)
	select {
	case b.MetricCh <- metricEntry{agentAddr, modelName, duration, true}:
	default:
	}

	metrics.InferenceRequestsTotal.WithLabelValues(modelName, agentID, "success").Inc()
	metrics.InferenceLatency.WithLabelValues(modelName, agentID).Observe(duration.Seconds())
}

func extractUsage(body []byte) (input, output int) {
	var full struct {
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
		Usage           *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &full); err == nil {
		if full.PromptEvalCount > 0 || full.EvalCount > 0 {
			return full.PromptEvalCount, full.EvalCount
		}
		if full.Usage != nil {
			return full.Usage.PromptTokens, full.Usage.CompletionTokens
		}
	}

	for i := len(body) - 1; i >= 0; i-- {
		if body[i] == '}' {
			for j := i - 1; j >= 0 && i-j < 2048; j-- {
				if body[j] == '{' {
					var partial struct {
						PromptEvalCount int `json:"prompt_eval_count"`
						EvalCount       int `json:"eval_count"`
						Usage           *struct {
							PromptTokens     int `json:"prompt_tokens"`
							CompletionTokens int `json:"completion_tokens"`
						} `json:"usage"`
					}
					if err := json.Unmarshal(body[j:i+1], &partial); err == nil {
						if partial.PromptEvalCount > 0 || partial.EvalCount > 0 {
							return partial.PromptEvalCount, partial.EvalCount
						}
						if partial.Usage != nil {
							return partial.Usage.PromptTokens, partial.Usage.CompletionTokens
						}
					}
				}
			}
		}
	}
	return 0, 0
}

func (b *Balancer) recordError(addr string, reason string) {
	b.State.Do(func(s *ClusterState) {
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
	b.State.Do(func(s *ClusterState) {
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
