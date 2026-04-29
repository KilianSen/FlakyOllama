package protocols

import (
	"FlakyOllama/pkg/shared/hash"
	"FlakyOllama/pkg/shared/models"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIAdapter struct {
	streaming bool
}

func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{}
}

func (a *OpenAIAdapter) TranslateRequest(r *http.Request) ([]byte, string, string, error) {
	var openAIReq models.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&openAIReq); err != nil {
		return nil, "", "", err
	}

	a.streaming = openAIReq.Stream

	// Normalize model name (strip "a." prefix if present)
	model := strings.TrimPrefix(openAIReq.Model, "a.")

	// Map to internal ChatRequest
	chatReq := models.ChatRequest{
		Model:   model,
		Stream:  openAIReq.Stream,
		Options: openAIReq.Options,
	}

	for _, msg := range openAIReq.Messages {
		chatReq.Messages = append(chatReq.Messages, models.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	contextHash := ""
	if len(chatReq.Messages) > 0 {
		contextHash = hash.ComputeHash(chatReq.Messages[len(chatReq.Messages)-1].Content)
	}

	body, err := json.Marshal(chatReq)
	return body, model, contextHash, err
}

func (a *OpenAIAdapter) TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	if a.streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		return a.translateStreamingResponse(w, agentBody)
	}
	return a.translateNonStreamingResponse(w, agentBody)
}

func (a *OpenAIAdapter) translateStreamingResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	scanner := bufio.NewScanner(agentBody)
	var lastInput, lastOutput int
	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ollamaResp models.ChatResponse
		if err := json.Unmarshal(line, &ollamaResp); err != nil {
			continue
		}

		openAIResp := models.OpenAIChatResponse{
			ID:      reqID,
			Object:  "chat.completion.chunk",
			Created: ollamaResp.CreatedAt.Unix(),
			Model:   ollamaResp.Model,
			Choices: []models.OpenAIChoice{
				{
					Index: 0,
					Delta: &models.OpenAIMessage{
						Role:    ollamaResp.Message.Role,
						Content: ollamaResp.Message.Content,
					},
				},
			},
		}

		if ollamaResp.Done {
			finishReason := "stop"
			openAIResp.Choices[0].FinishReason = &finishReason
		}

		data, _ := json.Marshal(openAIResp)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		if ollamaResp.Done {
			lastInput, lastOutput = extractUsageFromRaw(line)
			break
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	return lastInput, lastOutput, scanner.Err()
}

func (a *OpenAIAdapter) translateNonStreamingResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	var lastInput, lastOutput int
	var fullContent strings.Builder
	var lastModel string
	var lastCreated int64
	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	scanner := bufio.NewScanner(agentBody)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ollamaResp models.ChatResponse
		if err := json.Unmarshal(line, &ollamaResp); err != nil {
			continue
		}

		fullContent.WriteString(ollamaResp.Message.Content)
		lastModel = ollamaResp.Model
		lastCreated = ollamaResp.CreatedAt.Unix()

		if ollamaResp.Done {
			lastInput, lastOutput = extractUsageFromRaw(line)
		}
	}

	openAIResp := models.OpenAIChatResponse{
		ID:      reqID,
		Object:  "chat.completion",
		Created: lastCreated,
		Model:   lastModel,
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: &models.OpenAIMessage{
					Role:    "assistant",
					Content: fullContent.String(),
				},
				FinishReason: strPtr("stop"),
			},
		},
		Usage: &models.OpenAIUsage{
			PromptTokens:     lastInput,
			CompletionTokens: lastOutput,
			TotalTokens:      lastInput + lastOutput,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAIResp)
	return lastInput, lastOutput, scanner.Err()
}

type OpenAIEmbeddingAdapter struct {
	inputText string
}

func NewOpenAIEmbeddingAdapter() *OpenAIEmbeddingAdapter {
	return &OpenAIEmbeddingAdapter{}
}

func (a *OpenAIEmbeddingAdapter) TranslateRequest(r *http.Request) ([]byte, string, string, error) {
	var openAIReq models.OpenAIEmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&openAIReq); err != nil {
		return nil, "", "", err
	}

	model := strings.TrimPrefix(openAIReq.Model, "a.")

	// OpenAI 'input' can be string or array of strings
	var input interface{}
	switch v := openAIReq.Input.(type) {
	case string:
		input = v
		a.inputText = v
	case []interface{}:
		input = v
		// For hash/usage estimation, concatenate first few chars
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				a.inputText = s
			}
		}
	default:
		input = openAIReq.Input
	}

	ollamaReq := models.OllamaEmbeddingsRequest{
		Model: model,
		Input: input,
	}

	body, err := json.Marshal(ollamaReq)
	return body, model, "", err
}

func (a *OpenAIEmbeddingAdapter) TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	var ollamaResp models.OllamaEmbeddingsResponse
	if err := json.NewDecoder(agentBody).Decode(&ollamaResp); err != nil {
		return 0, 0, err
	}

	openAIResp := models.OpenAIEmbeddingResponse{
		Object: "list",
		Model:  ollamaResp.Model,
		Data:   make([]models.OpenAIEmbeddingData, len(ollamaResp.Embeddings)),
	}

	for i, emb := range ollamaResp.Embeddings {
		openAIResp.Data[i] = models.OpenAIEmbeddingData{
			Object:    "embedding",
			Embedding: emb,
			Index:     i,
		}
	}

	// Usage estimation for embeddings if not provided by Ollama
	inputTokens := len(a.inputText) / 4
	if inputTokens == 0 && len(a.inputText) > 0 {
		inputTokens = 1
	}
	openAIResp.Usage = models.OpenAIUsage{
		PromptTokens: inputTokens,
		TotalTokens:  inputTokens,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAIResp)
	return inputTokens, 0, nil
}

func extractUsageFromRaw(line []byte) (input, output int) {
	var usage struct {
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := json.Unmarshal(line, &usage); err == nil {
		return usage.PromptEvalCount, usage.EvalCount
	}
	return 0, 0
}

func strPtr(s string) *string {
	return &s
}
