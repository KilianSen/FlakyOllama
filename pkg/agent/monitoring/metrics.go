package monitoring

import (
	"FlakyOllama/pkg/shared/models"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	ewmaAlpha = 0.15 // smoothing factor — lower = more stable, higher = more reactive
	p95Window = 200  // samples kept for percentile computation
)

type modelStats struct {
	mu sync.Mutex

	requestCount int64
	errorCount   int64

	// EWMA accumulators
	avgTPS        float64
	avgTTFTMs     float64
	avgDurationMs float64
	avgInputTok   float64
	avgOutputTok  float64

	// Sliding window of durations for p95
	durations []float64
	lastUsed  time.Time
}

// ModelMetricsTracker maintains per-model performance statistics.
type ModelMetricsTracker struct {
	mu    sync.RWMutex
	stats map[string]*modelStats
}

func NewModelMetricsTracker() *ModelMetricsTracker {
	return &ModelMetricsTracker{stats: make(map[string]*modelStats)}
}

// RequestSample holds the measured values for a single completed inference request.
type RequestSample struct {
	Model        string
	TTFTMs       float64 // time-to-first-token in milliseconds
	DurationMs   float64 // total wall-clock duration in milliseconds
	InputTokens  int64
	OutputTokens int64
	Error        bool
}

// Record stores a completed request sample into the tracker.
func (t *ModelMetricsTracker) Record(s RequestSample) {
	t.mu.Lock()
	st, ok := t.stats[s.Model]
	if !ok {
		st = &modelStats{}
		t.stats[s.Model] = st
	}
	t.mu.Unlock()

	st.mu.Lock()
	defer st.mu.Unlock()

	st.requestCount++
	st.lastUsed = time.Now()

	if s.Error {
		st.errorCount++
		return
	}

	tps := 0.0
	if s.DurationMs > 0 && s.OutputTokens > 0 {
		tps = float64(s.OutputTokens) / (s.DurationMs / 1000.0)
	}

	ewma := func(old, next float64) float64 {
		if old == 0 {
			return next
		}
		return ewmaAlpha*next + (1-ewmaAlpha)*old
	}

	st.avgTPS = ewma(st.avgTPS, tps)
	st.avgTTFTMs = ewma(st.avgTTFTMs, s.TTFTMs)
	st.avgDurationMs = ewma(st.avgDurationMs, s.DurationMs)
	st.avgInputTok = ewma(st.avgInputTok, float64(s.InputTokens))
	st.avgOutputTok = ewma(st.avgOutputTok, float64(s.OutputTokens))

	st.durations = append(st.durations, s.DurationMs)
	if len(st.durations) > p95Window {
		st.durations = st.durations[len(st.durations)-p95Window:]
	}
}

// Snapshot returns an immutable capability map for all tracked models.
func (t *ModelMetricsTracker) Snapshot() map[string]models.ModelCapabilityStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make(map[string]models.ModelCapabilityStats, len(t.stats))
	for model, st := range t.stats {
		st.mu.Lock()

		errorRate := 0.0
		if st.requestCount > 0 {
			errorRate = float64(st.errorCount) / float64(st.requestCount)
		}

		out[model] = models.ModelCapabilityStats{
			RequestCount:    st.requestCount,
			ErrorCount:      st.errorCount,
			ErrorRate:       errorRate,
			AvgTPS:          round2(st.avgTPS),
			AvgTTFTMs:       round2(st.avgTTFTMs),
			AvgDurationMs:   round2(st.avgDurationMs),
			AvgInputTokens:  round2(st.avgInputTok),
			AvgOutputTokens: round2(st.avgOutputTok),
			P95DurationMs:   round2(percentile(st.durations, 0.95)),
			LastUsedAt:      st.lastUsed.UTC().Format(time.RFC3339),
		}
		st.mu.Unlock()
	}
	return out
}

func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(idx-float64(lo))
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
