package adapters

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Translator handles request/response format conversion between an external
// protocol (e.g. OpenAI) and the internal Ollama format.
type Translator interface {
	TranslateRequest(r *http.Request) (body []byte, model string, contextHash string, err error)
	TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (input int, output int, err error)
}

// Adapter is a protocol plugin: it translates requests/responses and registers
// its own HTTP routes. Implement this to add a new API protocol.
type Adapter interface {
	Translator
	RegisterRoutes(router *chi.Mux, d Dispatcher)
}

// AvailableModel is returned by Dispatcher.ListModels.
type AvailableModel struct {
	Name      string
	IsVirtual bool
}

// Dispatcher is the subset of balancer.Balancer that protocol adapters need.
// Defined here to avoid a circular import (balancer/proxy.go imports adapters).
type Dispatcher interface {
	AuthMiddleware(next http.Handler) http.Handler
	InferenceQuotaMiddleware(next http.Handler) http.Handler
	// DispatchInference routes a translated request to an agent and writes the
	// translated response. Pass nil translator for a raw Ollama-format passthrough.
	DispatchInference(w http.ResponseWriter, r *http.Request, agentPath string, body []byte, model, contextHash string, allowHedging bool, t Translator)
	// ListModels returns all models known to the cluster, including virtual ones.
	ListModels() []AvailableModel
}
