package balancer

import (
	"FlakyOllama/pkg/balancer/protocols"
	"FlakyOllama/pkg/shared/models"
	"net/http"
)

func (b *Balancer) HandleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	adapter := protocols.NewOpenAIAdapter()
	body, model, contextHash, err := adapter.TranslateRequest(r)
	if err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	priority := b.getRequestPriority(r)
	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

	// AGENT MAPPING: Use the internal /chat proxy (Ollama format)
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), model, "/chat", body, r.RemoteAddr, true, priority, contextHash)
	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	b.finalizeProxyWithAdapter(w, resp, agentAddr, model, r, surge, adapter)
}

func (b *Balancer) HandleOpenAIEmbeddings(w http.ResponseWriter, r *http.Request) {
	adapter := protocols.NewOpenAIEmbeddingAdapter()
	body, model, contextHash, err := adapter.TranslateRequest(r)
	if err != nil {
		b.jsonError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	// AGENT MAPPING: Use the internal /api/embeddings proxy
	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), model, "/api/embeddings", body, r.RemoteAddr, false, 10, contextHash)
	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	b.finalizeProxyWithAdapter(w, resp, agentAddr, model, r, 1.0, adapter)
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
