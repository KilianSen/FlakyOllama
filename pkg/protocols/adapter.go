package protocols

import (
	"io"
	"net/http"
)

// Adapter defines the interface for translating external API formats
// (e.g., OpenAI, Anthropic) to the internal Ollama format and back.
type Adapter interface {
	// TranslateRequest parses the incoming client request and returns the
	// JSON body for the agent, the target model name, and a context hash.
	TranslateRequest(r *http.Request) (internalBody []byte, model string, contextHash string, err error)

	// TranslateResponse reads the agent's (Ollama-style) response stream,
	// translates it to the client's format, and writes it to the response writer.
	// It returns the prompt and completion token counts.
	TranslateResponse(w http.ResponseWriter, agentBody io.Reader) (input int, output int, err error)
}
