package openai

import (
	"FlakyOllama/pkg/balancer/adapters"
	"FlakyOllama/pkg/balancer/hash"
	"FlakyOllama/pkg/balancer/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Adapter struct {
	streaming bool
}

func (a *Adapter) RegisterRoutes(r *chi.Mux, d adapters.Dispatcher) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(d.AuthMiddleware)
		r.Get("/models", handleModels(d))
		r.Group(func(r chi.Router) {
			r.Use(d.InferenceQuotaMiddleware)
			r.Post("/chat/completions", handleChat(d))
			r.Post("/completions", handleCompletions(d))
			r.Post("/embeddings", handleEmbeddings(d))
		})
	})
}

func NewOpenAIAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) TranslateRequest(r *http.Request) ([]byte, string, string, error) {
	var openAIReq ChatRequest
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

func (a *Adapter) TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	if a.streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// WriteHeader(StatusOK) should be called by the caller if they want,
		// but proxy.go does it before calling us or we do it.
		// Actually, proxy.go was updated to let adapter handle it.
		w.WriteHeader(http.StatusOK)
		return a.translateStreamingResponse(w, agentBody)
	}
	return a.translateNonStreamingResponse(w, agentBody)
}

func (a *Adapter) translateStreamingResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	decoder := json.NewDecoder(agentBody)
	var lastInput, lastOutput int
	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	for {
		var ollamaResp models.ChatResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			return lastInput, lastOutput, err
		}

		if ollamaResp.PromptEval > 0 {
			lastInput = ollamaResp.PromptEval
		}
		if ollamaResp.EvalCount > 0 {
			lastOutput = ollamaResp.EvalCount
		}

		openAIResp := ChatResponse{
			ID:      reqID,
			Object:  "chat.completion.chunk",
			Created: ollamaResp.CreatedAt.Unix(),
			Model:   ollamaResp.Model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: &Message{
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
			break
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	return lastInput, lastOutput, nil
}

func (a *Adapter) translateNonStreamingResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	decoder := json.NewDecoder(agentBody)
	var lastInput, lastOutput int
	var fullContent strings.Builder
	var lastModel string
	var lastCreated int64
	reqID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	for {
		var ollamaResp models.ChatResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			// If we got some content but then an error, we should probably still try to return what we have?
			// But a 502/503 is likely a connection drop.
			return lastInput, lastOutput, err
		}

		if ollamaResp.PromptEval > 0 {
			lastInput = ollamaResp.PromptEval
		}
		if ollamaResp.EvalCount > 0 {
			lastOutput = ollamaResp.EvalCount
		}

		fullContent.WriteString(ollamaResp.Message.Content)
		if ollamaResp.Model != "" {
			lastModel = ollamaResp.Model
		}
		if !ollamaResp.CreatedAt.IsZero() {
			lastCreated = ollamaResp.CreatedAt.Unix()
		}

		if ollamaResp.Done {
			break
		}
	}

	if lastCreated == 0 {
		lastCreated = time.Now().Unix()
	}

	openAIResp := ChatResponse{
		ID:      reqID,
		Object:  "chat.completion",
		Created: lastCreated,
		Model:   lastModel,
		Choices: []Choice{
			{
				Index: 0,
				Message: &Message{
					Role:    "assistant",
					Content: fullContent.String(),
				},
				FinishReason: strPtr("stop"),
			},
		},
		Usage: &Usage{
			PromptTokens:     lastInput,
			CompletionTokens: lastOutput,
			TotalTokens:      lastInput + lastOutput,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openAIResp)
	return lastInput, lastOutput, nil
}

type OpenAIEmbeddingAdapter struct {
	inputText string
}

func NewOpenAIEmbeddingAdapter() *OpenAIEmbeddingAdapter {
	return &OpenAIEmbeddingAdapter{}
}

func (a *OpenAIEmbeddingAdapter) TranslateRequest(r *http.Request) ([]byte, string, string, error) {
	var openAIReq EmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&openAIReq); err != nil {
		return nil, "", "", err
	}

	model := strings.TrimPrefix(openAIReq.Model, "a.")

	var input interface{}
	switch v := openAIReq.Input.(type) {
	case string:
		input = v
		a.inputText = v
	case []interface{}:
		input = v
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

	contextHash := hash.ComputeHash(a.inputText)

	body, err := json.Marshal(ollamaReq)
	return body, model, contextHash, err
}

func (a *OpenAIEmbeddingAdapter) TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (int, int, error) {
	var ollamaResp models.OllamaEmbeddingsResponse
	if err := json.NewDecoder(agentBody).Decode(&ollamaResp); err != nil {
		return 0, 0, err
	}

	openAIResp := EmbeddingResponse{
		Object: "list",
		Model:  ollamaResp.Model,
		Data:   make([]EmbeddingData, len(ollamaResp.Embeddings)),
	}

	for i, emb := range ollamaResp.Embeddings {
		openAIResp.Data[i] = EmbeddingData{
			Object:    "embedding",
			Embedding: emb,
			Index:     i,
		}
	}

	inputTokens := len(a.inputText) / 4
	if inputTokens == 0 && len(a.inputText) > 0 {
		inputTokens = 1
	}
	openAIResp.Usage = Usage{
		PromptTokens: inputTokens,
		TotalTokens:  inputTokens,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(openAIResp)
	return inputTokens, 0, nil
}

func strPtr(s string) *string {
	return &s
}
