package balancer

import (
	models2 "FlakyOllama/pkg/balancer/models"
	"FlakyOllama/pkg/shared/logging"
	"context"
	"encoding/json"
	"fmt"
)

func (b *Balancer) ExecutePipeline(ctx context.Context, initial models2.ChatRequest, vConfig models2.VirtualModelConfig, clientIP string) (string, error) {
	currentOutput := ""
	for i, step := range vConfig.Steps {
		logging.Global.Infof("Executing pipeline step %d/%d: %s on %s", i+1, len(vConfig.Steps), step.Action, step.Model)

		prompt := step.SystemPrompt + "\n\n" + currentOutput
		if i == 0 {
			// First step uses the actual user message
			if len(initial.Messages) > 0 {
				prompt = step.SystemPrompt + "\n\n" + initial.Messages[len(initial.Messages)-1].Content
			}
		}

		req := models2.ChatRequest{
			Model: step.Model,
			Messages: []models2.ChatMessage{
				{Role: "user", Content: prompt},
			},
			Stream: false,
		}

		body, _ := json.Marshal(req)
		resp, _, _, err := b.DoHedgedRequest(ctx, step.Model, "/chat", body, clientIP, false, 10, "")
		if err != nil {
			return "", fmt.Errorf("pipeline step %d failed: %w", i, err)
		}
		defer resp.Body.Close()

		var res models2.ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return "", fmt.Errorf("failed to decode pipeline step %d: %w", i, err)
		}
		currentOutput = res.Message.Content
	}

	return currentOutput, nil
}
