package models

// ModelCapabilityStats holds per-model performance statistics measured locally by the agent.
type ModelCapabilityStats struct {
	RequestCount    int64   `json:"request_count"`
	ErrorCount      int64   `json:"error_count"`
	ErrorRate       float64 `json:"error_rate"`
	AvgTPS          float64 `json:"avg_tps"`         // output tokens / second
	AvgTTFTMs       float64 `json:"avg_ttft_ms"`     // time to first token (ms)
	AvgDurationMs   float64 `json:"avg_duration_ms"` // wall-clock request duration (ms)
	AvgInputTokens  float64 `json:"avg_input_tokens"`
	AvgOutputTokens float64 `json:"avg_output_tokens"`
	P95DurationMs   float64 `json:"p95_duration_ms"`
	LastUsedAt      string  `json:"last_used_at,omitempty"`
}
