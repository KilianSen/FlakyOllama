package balancer

import (
	"FlakyOllama/pkg/models"
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (b *Balancer) HandleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	var oaiReq models.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&oaiReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Map OpenAI to Ollama/Internal
	ollamaReq := models.ChatRequest{
		Model:   oaiReq.Model,
		Stream:  oaiReq.Stream,
		Options: oaiReq.Options,
	}
	for _, m := range oaiReq.Messages {
		ollamaReq.Messages = append(ollamaReq.Messages, models.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	b.Mu.Lock()
	b.PendingRequests[ollamaReq.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[ollamaReq.Model]--
		b.Mu.Unlock()
	}()

	body, _ := json.Marshal(ollamaReq)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), ollamaReq.Model, "/chat", body, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Concurrency Tracking: start
	if agentAddr != "" {
		b.Mu.Lock()
		b.NodeWorkloads[agentAddr]++
		b.Mu.Unlock()
		defer func() {
			b.Mu.Lock()
			b.NodeWorkloads[agentAddr]--
			b.Mu.Unlock()
		}()
	}

	if !oaiReq.Stream {
		b.handleOpenAIChatNonStream(w, resp, oaiReq.Model, agentAddr)
	} else {
		b.handleOpenAIChatStream(w, resp, oaiReq.Model, agentAddr)
	}
}

func (b *Balancer) handleOpenAIChatNonStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	// Ollama chat non-stream is actually many chunks if we aren't careful,
	// but usually it's one big JSON if stream=false.
	var ollamaResp struct {
		Message models.ChatMessage `json:"message"`
	}

	// We might need to handle multiple JSON objects if it was proxied as a stream but requested as non-stream.
	// But our Agent HandleChat respects the stream flag.
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		http.Error(w, "Failed to decode agent response", http.StatusInternalServerError)
		return
	}

	oaiResp := models.OpenAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: &models.OpenAIMessage{
					Role:    ollamaResp.Message.Role,
					Content: ollamaResp.Message.Content,
				},
			},
		},
	}

	b.recordSuccess(agentAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiResp)
}

func (b *Balancer) handleOpenAIChatStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	scanner := bufio.NewScanner(resp.Body)
	id := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ollamaChunk struct {
			Message models.ChatMessage `json:"message"`
			Done    bool               `json:"done"`
		}
		if err := json.Unmarshal(line, &ollamaChunk); err != nil {
			continue
		}

		oaiChunk := models.OpenAIChatResponse{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []models.OpenAIChoice{
				{
					Index: 0,
					Delta: &models.OpenAIMessage{
						Role:    ollamaChunk.Message.Role,
						Content: ollamaChunk.Message.Content,
					},
				},
			},
		}

		chunkBody, _ := json.Marshal(oaiChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBody))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		if ollamaChunk.Done {
			break
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	b.recordSuccess(agentAddr)
}

func (b *Balancer) HandleOpenAICompletions(w http.ResponseWriter, r *http.Request) {
	var oaiReq models.OpenAICompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&oaiReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ollamaReq := models.InferenceRequest{
		Model:   oaiReq.Model,
		Prompt:  oaiReq.Prompt,
		Stream:  oaiReq.Stream,
		Options: oaiReq.Options,
	}

	b.Mu.Lock()
	b.PendingRequests[ollamaReq.Model]++
	b.Mu.Unlock()
	defer func() {
		b.Mu.Lock()
		b.PendingRequests[ollamaReq.Model]--
		b.Mu.Unlock()
	}()

	body, _ := json.Marshal(ollamaReq)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), ollamaReq.Model, "/inference", body, r.RemoteAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Concurrency Tracking: start
	if agentAddr != "" {
		b.Mu.Lock()
		b.NodeWorkloads[agentAddr]++
		b.Mu.Unlock()
		defer func() {
			b.Mu.Lock()
			b.NodeWorkloads[agentAddr]--
			b.Mu.Unlock()
		}()
	}

	if !oaiReq.Stream {
		b.handleOpenAICompletionNonStream(w, resp, oaiReq.Model, agentAddr)
	} else {
		b.handleOpenAICompletionStream(w, resp, oaiReq.Model, agentAddr)
	}
}

func (b *Balancer) handleOpenAICompletionNonStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		http.Error(w, "Failed to decode agent response", http.StatusInternalServerError)
		return
	}

	oaiResp := models.OpenAIChatResponse{
		ID:      fmt.Sprintf("cmpl-%d", time.Now().Unix()),
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.OpenAIChoice{
			{
				Index: 0,
				Message: &models.OpenAIMessage{
					Content: ollamaResp.Response,
				},
			},
		},
	}

	b.recordSuccess(agentAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiResp)
}

func (b *Balancer) handleOpenAICompletionStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	scanner := bufio.NewScanner(resp.Body)
	id := fmt.Sprintf("cmpl-%d", time.Now().Unix())

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ollamaChunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := json.Unmarshal(line, &ollamaChunk); err != nil {
			continue
		}

		oaiChunk := models.OpenAIChatResponse{
			ID:      id,
			Object:  "text_completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []models.OpenAIChoice{
				{
					Index: 0,
					Delta: &models.OpenAIMessage{
						Content: ollamaChunk.Response,
					},
				},
			},
		}

		chunkBody, _ := json.Marshal(oaiChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBody))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		if ollamaChunk.Done {
			break
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	b.recordSuccess(agentAddr)
}

func (b *Balancer) HandleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	modelMap := make(map[string]bool)
	for _, agent := range b.Agents {
		for _, m := range agent.ActiveModels {
			modelMap[m] = true
		}
	}

	var oaiModels []models.OpenAIModel
	for m := range modelMap {
		oaiModels = append(oaiModels, models.OpenAIModel{
			ID:      m,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "flakyollama",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.OpenAIModelList{
		Object: "list",
		Data:   oaiModels,
	})
}
