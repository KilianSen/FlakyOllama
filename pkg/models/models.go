package models

import "time"

// NodeStatus represents the current state of an Agent node.
type NodeStatus struct {
	ID           string    `json:"id"`
	Address      string    `json:"address"`
	CPUUsage     float64   `json:"cpu_usage"`     // Percentage
	MemoryUsage  float64   `json:"memory_usage"`  // Percentage
	VRAMTotal    uint64    `json:"vram_total"`    // Bytes
	VRAMUsed     uint64    `json:"vram_used"`     // Bytes
	GPUTemperature float64 `json:"gpu_temp"`      // Celsius
	ActiveModels []string  `json:"active_models"` // List of currently loaded models
	LastSeen     time.Time `json:"last_seen"`
}

// ModelRequirement defines the hardware needs for a model.
type ModelRequirement struct {
	MinVRAM uint64 `json:"min_vram"` // Bytes
}

// InferenceRequest is a simplified request for an LLM task.
type InferenceRequest struct {
	Model  string                 `json:"model"`
	Prompt string                 `json:"prompt"`
	Stream bool                   `json:"stream"`
	Options map[string]interface{} `json:"options"`
}

// InferenceResponse is the result of an inference task.
type InferenceResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// RegisterRequest is sent by an Agent to the Balancer.
type RegisterRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}
