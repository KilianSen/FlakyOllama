package openai

import (
	"FlakyOllama/pkg/balancer/adapters"
	"FlakyOllama/pkg/balancer/hash"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

func handleChat(d adapters.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := &Adapter{}
		body, model, ctxHash, err := a.TranslateRequest(r)
		if err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		d.DispatchInference(w, r, "/api/chat", body, model, ctxHash, a.streaming, a)
	}
}

func handleCompletions(d adapters.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request", http.StatusBadRequest)
			return
		}
		var req CompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		model := strings.TrimPrefix(req.Model, "a.")
		ctxHash := hash.ComputeHash(req.Prompt)
		// Pass nil translator — response is returned in raw Ollama generate format.
		d.DispatchInference(w, r, "/api/generate", body, model, ctxHash, req.Stream, nil)
	}
}

func handleEmbeddings(d adapters.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := &OpenAIEmbeddingAdapter{}
		body, model, ctxHash, err := a.TranslateRequest(r)
		if err != nil {
			http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
			return
		}
		d.DispatchInference(w, r, "/api/embeddings", body, model, ctxHash, false, a)
	}
}

func handleModels(d adapters.Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		available := d.ListModels()
		list := ModelList{
			Object: "list",
			Data:   make([]Model, 0, len(available)),
		}
		for _, m := range available {
			ownedBy := "flakyollama"
			if m.IsVirtual {
				ownedBy = "flakyollama/virtual"
			}
			list.Data = append(list.Data, Model{
				ID:      m.Name,
				Object:  "model",
				Created: 1686935002,
				OwnedBy: ownedBy,
			})
		}
		sort.Slice(list.Data, func(i, j int) bool {
			return list.Data[i].ID < list.Data[j].ID
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}
