package balancer

import (
	"FlakyOllama/pkg/shared/logging"
	"FlakyOllama/pkg/shared/models"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// ResolveVirtualModel handles the initial resolution of a virtual model name to a real model.
func (b *Balancer) ResolveVirtualModel(modelName string) (string, models.VirtualModelConfig, bool) {
	config, ok := b.Config.VirtualModels[modelName]
	if !ok {
		return modelName, models.VirtualModelConfig{}, false
	}

	switch config.Type {
	case "arena":
		if len(config.Targets) > 0 {
			target := config.Targets[rand.Intn(len(config.Targets))]
			logging.Global.Infof("Virtual model %s (arena) resolved to %s", modelName, target)
			return target, config, true
		}
	case "metric":
		target := b.selectByMetric(config.Targets, config.Strategy)
		logging.Global.Infof("Virtual model %s (metric:%s) resolved to %s", modelName, config.Strategy, target)
		return target, config, true
	case "pipeline":
		// Pipelines are handled differently, but the first target is usually the default worker
		if len(config.Targets) > 0 {
			return config.Targets[0], config, true
		}
	}

	return modelName, config, ok
}

func (b *Balancer) selectByMetric(targets []string, strategy string) string {
	if len(targets) == 0 {
		return ""
	}

	b.perfMu.RLock()
	defer b.perfMu.RUnlock()

	bestTarget := targets[0]
	
	switch strategy {
	case "fastest":
		minLatency := 9999.0
		for _, t := range targets {
			// We look at the best performing node for this model
			// This is a simplification; we could look at average across cluster
			snapshot := b.State.GetSnapshot()
			for _, a := range snapshot.Agents {
				perf, ok := b.PerfCache[a.ID+":"+t]
				if ok && perf.AvgLatency > 0 && perf.AvgLatency < minLatency {
					minLatency = perf.AvgLatency
					bestTarget = t
				}
			}
		}
	case "cheapest":
		minCost := 9999.0
		for _, t := range targets {
			cost := 1.0
			if f, ok := b.Config.ModelCostFactors[t]; ok {
				cost = f
			}
			if cost < minCost {
				minCost = cost
				bestTarget = t
			}
		}
	case "most_reliable":
		maxSuccess := -1.0
		for _, t := range targets {
			snapshot := b.State.GetSnapshot()
			for _, a := range snapshot.Agents {
				perf, ok := b.PerfCache[a.ID+":"+t]
				if ok && perf.SuccessRate > maxSuccess {
					maxSuccess = perf.SuccessRate
					bestTarget = t
				}
			}
		}
	}

	return bestTarget
}

// ExecutePipeline implements the recursive/multi-stage logic.
// For now, this is a placeholder for the full recursive state machine.
func (b *Balancer) ExecutePipeline(ctx context.Context, req models.ChatRequest, config models.VirtualModelConfig) (string, error) {
	// 1. Initial Generation
	// In a real implementation, we would loop through config.Steps
	// For simplicity in this phase, let's implement a hardcoded "Check & Retry" loop
	// if the pipeline type is "pipeline" and has steps.
	
	currentMessages := req.Messages
	maxGlobalRetries := 3
	
	for i := 0; i < maxGlobalRetries; i++ {
		// Use the first target as the primary worker
		workerModel := config.Targets[0]
		
		// 1. Generate
		workerReq := models.ChatRequest{
			Model:    workerModel,
			Messages: currentMessages,
			Stream:   false, // We must buffer for pipelines to allow for checks
			Options:  req.Options,
		}
		
		body, _ := json.Marshal(workerReq)
		resp, _, _, err := b.DoHedgedRequest(ctx, workerModel, "/chat", body, "127.0.0.1", true, req.Priority, "")
		if err != nil {
			return "", err
		}
		
		var ollamaResp struct {
			Message models.ChatMessage `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&ollamaResp)
		resp.Body.Close()
		
		workerOutput := ollamaResp.Message.Content
		
		// 2. Check (if judge model is configured)
		if config.JudgeModel != "" {
			logging.Global.Infof("Pipeline: Judge %s evaluating output from %s (attempt %d)", config.JudgeModel, workerModel, i+1)
			
			judgePrompt := fmt.Sprintf("Evaluate the following response for correctness and quality.\nUser Prompt: %s\n\nModel Response: %s\n\nIf the response is good, reply with 'PASS'. If it needs correction, reply with 'FAIL: [REASON]'.", 
				req.Messages[len(req.Messages)-1].Content, workerOutput)
				
			judgeReq := models.ChatRequest{
				Model: config.JudgeModel,
				Messages: []models.ChatMessage{
					{Role: "user", Content: judgePrompt},
				},
			}
			
			jBody, _ := json.Marshal(judgeReq)
			jResp, _, _, jErr := b.DoHedgedRequest(ctx, config.JudgeModel, "/chat", jBody, "127.0.0.1", true, 0, "")
			if jErr == nil {
				var jOllamaResp struct {
					Message models.ChatMessage `json:"message"`
				}
				json.NewDecoder(jResp.Body).Decode(&jOllamaResp)
				jResp.Body.Close()
				
				judgeVerdict := strings.ToUpper(jOllamaResp.Message.Content)
				if strings.Contains(judgeVerdict, "PASS") {
					logging.Global.Infof("Pipeline: Judge passed output")
					return workerOutput, nil
				} else {
					logging.Global.Warnf("Pipeline: Judge failed output: %s", judgeVerdict)
					// Recursive step: feed the failure back to the worker
					currentMessages = append(currentMessages, models.ChatMessage{Role: "assistant", Content: workerOutput})
					currentMessages = append(currentMessages, models.ChatMessage{Role: "user", Content: "Your previous answer was rejected by the grader: " + judgeVerdict + ". Please try again and fix the issues."})
					continue
				}
			}
		}
		
		return workerOutput, nil
	}
	
	return "", fmt.Errorf("pipeline failed after max retries")
}
