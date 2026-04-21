package balancer

import (
	"FlakyOllama/pkg/balancer/state"
	"FlakyOllama/pkg/shared/models"
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

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
		return
	}

	b.State.Do(func(s *state.ClusterState) {
		s.PendingRequests[ollamaReq.Model]++
	})
	defer func() {
		b.State.Do(func(s *state.ClusterState) {
			s.PendingRequests[ollamaReq.Model]--
		})
	}()

	body, _ := json.Marshal(ollamaReq)
	priority := b.getRequestPriority(r)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), ollamaReq.Model, "/chat", body, r.RemoteAddr, ollamaReq.AllowHedging, priority)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if !oaiReq.Stream {
		b.handleOpenAIChatNonStream(w, resp, oaiReq.Model, agentAddr)
	} else {
		b.handleOpenAIChatStream(w, resp, oaiReq.Model, agentAddr)
	}
}

func (b *Balancer) handleOpenAIChatNonStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	var ollamaResp struct {
		Message         models.ChatMessage `json:"message"`
		PromptEvalCount int                `json:"prompt_eval_count"`
		EvalCount       int                `json:"eval_count"`
	}

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

	// Capture usage
	if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
		agentID := agentAddr
		rewardKey := ""
		b.State.Do(func(s *state.ClusterState) {
			if a, ok := s.Agents[agentAddr]; ok {
				agentID = a.ID
				rewardKey = a.AgentKey
			}
		})
		// Surge pricing
		surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

		// Calculate reward (Agent)
		rFactor := 1.0
		if f, ok := b.Config.ModelRewardFactors[model]; ok {
			rFactor = f
		}
		reward := float64(ollamaResp.PromptEvalCount+ollamaResp.EvalCount) * rFactor * b.Config.GlobalRewardMultiplier * surge

		// Calculate cost (Client)
		cFactor := 1.0
		if f, ok := b.Config.ModelCostFactors[model]; ok {
			cFactor = f
		}
		cost := float64(ollamaResp.PromptEvalCount+ollamaResp.EvalCount) * cFactor * b.Config.GlobalCostMultiplier * surge

		trackingID := agentID
		if rewardKey != "" {
			trackingID = rewardKey
		}

		// Get client key
		clientKey, _ := r.Context().Value(auth.ContextKeyToken).(string)

		select {
		case b.TokenCh <- tokenUsageEntry{trackingID, model, ollamaResp.PromptEvalCount, ollamaResp.EvalCount, reward, cost, clientKey}:
		default:
		}
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
			Message         models.ChatMessage `json:"message"`
			Done            bool               `json:"done"`
			PromptEvalCount int                `json:"prompt_eval_count"`
			EvalCount       int                `json:"eval_count"`
		}
		if err := json.Unmarshal(line, &ollamaChunk); err != nil {
			continue
		}

		if ollamaChunk.PromptEvalCount > 0 || ollamaChunk.EvalCount > 0 {
			agentID := agentAddr
			rewardKey := ""
			b.State.Do(func(s *state.ClusterState) {
				if a, ok := s.Agents[agentAddr]; ok {
					agentID = a.ID
					rewardKey = a.AgentKey
				}
			})
			// Calculate reward (Agent)
			rFactor := 1.0
			if f, ok := b.Config.ModelRewardFactors[model]; ok {
				rFactor = f
			}
			reward := float64(ollamaChunk.PromptEvalCount+ollamaChunk.EvalCount) * rFactor * b.Config.GlobalRewardMultiplier

			// Calculate cost (Client)
			cFactor := 1.0
			if f, ok := b.Config.ModelCostFactors[model]; ok {
				cFactor = f
			}
			cost := float64(ollamaChunk.PromptEvalCount+ollamaChunk.EvalCount) * cFactor * b.Config.GlobalCostMultiplier

			trackingID := agentID
			if rewardKey != "" {
				trackingID = rewardKey
			}

			// Get client key
			clientKey, _ := r.Context().Value(auth.ContextKeyToken).(string)

			select {
			case b.TokenCh <- tokenUsageEntry{trackingID, model, ollamaChunk.PromptEvalCount, ollamaChunk.EvalCount, reward, cost, clientKey}:
			default:
			}
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

	// Load Shedding: Check if queue is at capacity
	if b.Queue.pq.Len() >= b.Config.MaxQueueDepth {
		http.Error(w, "Cluster saturated, too many requests queued", http.StatusTooManyRequests)
		return
	}

	b.State.Do(func(s *state.ClusterState) {
		s.PendingRequests[ollamaReq.Model]++
	})
	defer func() {
		b.State.Do(func(s *state.ClusterState) {
			s.PendingRequests[ollamaReq.Model]--
		})
	}()

	body, _ := json.Marshal(ollamaReq)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), ollamaReq.Model, "/inference", body, r.RemoteAddr, ollamaReq.AllowHedging, ollamaReq.Priority)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	if !oaiReq.Stream {
		b.handleOpenAICompletionNonStream(w, resp, oaiReq.Model, agentAddr)
	} else {
		b.handleOpenAICompletionStream(w, resp, oaiReq.Model, agentAddr)
	}
}

func (b *Balancer) handleOpenAICompletionNonStream(w http.ResponseWriter, resp *http.Response, model, agentAddr string) {
	var ollamaResp struct {
		Response        string `json:"response"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
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

	// Capture usage
	if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
		agentID := agentAddr
		rewardKey := ""
		b.State.Do(func(s *state.ClusterState) {
			if a, ok := s.Agents[agentAddr]; ok {
				agentID = a.ID
				rewardKey = a.AgentKey
			}
		})
		// Surge pricing
		surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

		// Calculate reward (Agent)
		rFactor := 1.0
		if f, ok := b.Config.ModelRewardFactors[model]; ok {
			rFactor = f
		}
		reward := float64(ollamaResp.PromptEvalCount+ollamaResp.EvalCount) * rFactor * b.Config.GlobalRewardMultiplier * surge

		// Calculate cost (Client)
		cFactor := 1.0
		if f, ok := b.Config.ModelCostFactors[model]; ok {
			cFactor = f
		}
		cost := float64(ollamaResp.PromptEvalCount+ollamaResp.EvalCount) * cFactor * b.Config.GlobalCostMultiplier * surge

		trackingID := agentID
		if rewardKey != "" {
			trackingID = rewardKey
		}

		// Get client key
		clientKey, _ := r.Context().Value(auth.ContextKeyToken).(string)

		select {
		case b.TokenCh <- tokenUsageEntry{trackingID, model, ollamaResp.PromptEvalCount, ollamaResp.EvalCount, reward, cost, clientKey}:
		default:
		}
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
			Response        string `json:"response"`
			Done            bool   `json:"done"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
		}
		if err := json.Unmarshal(line, &ollamaChunk); err != nil {
			continue
		}

		if ollamaChunk.PromptEvalCount > 0 || ollamaChunk.EvalCount > 0 {
			agentID := agentAddr
			rewardKey := ""
			b.State.Do(func(s *state.ClusterState) {
				if a, ok := s.Agents[agentAddr]; ok {
					agentID = a.ID
					rewardKey = a.AgentKey
				}
			})
			// Calculate reward (Agent)
			rFactor := 1.0
			if f, ok := b.Config.ModelRewardFactors[model]; ok {
				rFactor = f
			}
			reward := float64(ollamaChunk.PromptEvalCount+ollamaChunk.EvalCount) * rFactor * b.Config.GlobalRewardMultiplier

			// Calculate cost (Client)
			cFactor := 1.0
			if f, ok := b.Config.ModelCostFactors[model]; ok {
				cFactor = f
			}
			cost := float64(ollamaChunk.PromptEvalCount+ollamaChunk.EvalCount) * cFactor * b.Config.GlobalCostMultiplier

			trackingID := agentID
			if rewardKey != "" {
				trackingID = rewardKey
			}

			// Get client key
			clientKey, _ := r.Context().Value(auth.ContextKeyToken).(string)

			select {
			case b.TokenCh <- tokenUsageEntry{trackingID, model, ollamaChunk.PromptEvalCount, ollamaChunk.EvalCount, reward, cost, clientKey}:
			default:
			}
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
	snapshot := b.State.GetSnapshot()

	modelMap := make(map[string]bool)
	for _, agent := range snapshot.Agents {
		for _, m := range agent.ActiveModels {
			modelMap[m] = true
		}
		for _, m := range agent.LocalModels {
			modelMap[m.Model] = true
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

func (b *Balancer) HandleOpenAIEmbeddings(w http.ResponseWriter, r *http.Request) {
	var oaiReq models.OpenAIEmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&oaiReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Map OpenAI to Ollama
	ollamaReq := struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}{
		Model: oaiReq.Model,
		Input: oaiReq.Input,
	}

	body, _ := json.Marshal(ollamaReq)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), oaiReq.Model, "/embeddings", body, r.RemoteAddr, false, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	var ollamaResp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		http.Error(w, "Failed to decode agent response", http.StatusInternalServerError)
		return
	}

	oaiResp := models.OpenAIEmbeddingResponse{
		Object: "list",
		Model:  oaiReq.Model,
		Data: []models.OpenAIEmbeddingData{
			{
				Object:    "embedding",
				Embedding: ollamaResp.Embedding,
				Index:     0,
			},
		},
	}

	b.recordSuccess(agentAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiResp)
}
