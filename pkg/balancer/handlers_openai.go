package balancer

import (
	"FlakyOllama/pkg/shared/models"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (b *Balancer) HandleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	var req models.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Strip prefixes for OpenAI compatibility too
	req.Model = strings.TrimPrefix(req.Model, "a.")

	priority := b.getRequestPriority(r)
	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

	contextHash := ""
	if len(req.Messages) > 0 {
		contextHash = b.computeHash(req.Messages[len(req.Messages)-1].Content)
	}

	body, _ := json.Marshal(req)
	// AGENT MAPPING: /v1/chat/completions -> /chat
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), req.Model, "/chat", body, r.RemoteAddr, true, priority, contextHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	b.finalizeProxy(w, resp, agentAddr, req.Model, r, surge)
}

func (b *Balancer) HandleOpenAIEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req models.OpenAIEmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Model = strings.TrimPrefix(req.Model, "a.")

	body, _ := json.Marshal(req)
	// AGENT MAPPING: /v1/embeddings -> /api/embeddings (Ollama standard)
	resp, _, _, err := b.DoHedgedRequest(r.Context(), req.Model, "/api/embeddings", body, r.RemoteAddr, false, 10, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (b *Balancer) HandleV1Models(w http.ResponseWriter, r *http.Request) {
	snap := b.State.GetSnapshot()
	uniqueModels := make(map[string]bool)
	for _, node := range snap.Agents {
		if node.State != models.StateBroken && !node.Draining {
			for _, m := range node.LocalModels {
				uniqueModels[m.Name] = true
			}
		}
	}

	// Add Virtual Models to OpenAI list
	b.configMu.RLock()
	for m := range b.Config.VirtualModels {
		uniqueModels[m] = true
	}
	b.configMu.RUnlock()

	var list models.OpenAIModelList
	list.Object = "list"
	list.Data = make([]models.OpenAIModel, 0, len(uniqueModels))
	for m := range uniqueModels {
		list.Data = append(list.Data, models.OpenAIModel{
			ID:      m,
			Object:  "model",
			Created: 1686935002,
			OwnedBy: "flakyollama",
		})
	}

	b.jsonResponse(w, http.StatusOK, list)
}
