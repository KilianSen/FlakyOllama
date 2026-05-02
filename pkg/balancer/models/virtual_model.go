package models

type VirtualModelConfig struct {
	Type       string         `json:"type"`        // "pipeline", "arena", "metric"
	Strategy   string         `json:"strategy"`    // "fastest", "cheapest", "random"
	JudgeModel string         `json:"judge_model"` // If type is judge/pipeline
	Targets    []string       `json:"targets"`     // Real backing models
	Steps      []PipelineStep `json:"steps"`       // Execution flow
}

// IsRoutable returns true if the model is routable (has at least one target)
func (vm *VirtualModelConfig) IsRoutable() bool {
	return len(vm.Targets) > 0
}

type PipelineStep struct {
	Action       string            `json:"action"` // "generate", "classify", "check"
	Model        string            `json:"model"`
	SystemPrompt string            `json:"system_prompt"`
	MaxRetries   int               `json:"max_retries"`
	OnSuccess    string            `json:"on_success"` // "next", "return"
	OnFail       string            `json:"on_fail"`    // "retry", "fallback", "error"
	Routes       map[string]string `json:"routes"`     // If action is classify
}
