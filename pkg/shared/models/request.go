package models

type InferenceRequest struct {
	Model        string                 `json:"model"`
	Prompt       string                 `json:"prompt"`
	Options      map[string]interface{} `json:"options"`
	Stream       bool                   `json:"stream"`
	Priority     int                    `json:"priority"`
	AllowHedging bool                   `json:"allow_hedging"`
}

// RegisterRequest is sent by an Agent to the Balancer.
type RegisterRequest struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Tier     string `json:"tier"`
	HasGPU   bool   `json:"has_gpu"`
	GPUModel string `json:"gpu_model"`
}

type TelemetryResponse struct {
	Status           string   `json:"status"`
	PersistentModels []string `json:"persistent_models,omitempty"`
}
