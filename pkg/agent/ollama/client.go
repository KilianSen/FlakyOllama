package ollama

import (
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Request types for Ollama API
type OllamaDeleteRequest struct {
	Name string `json:"name"`
}

type OllamaPullRequest struct {
	Name string `json:"name"`
}

type OllamaUnloadRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	KeepAlive int    `json:"keep_alive"`
}

type OllamaShowRequest struct {
	Name string `json:"name"`
}

type OllamaEmbeddingsRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type OllamaCreateRequest struct {
	Name      string `json:"name"`
	Modelfile string `json:"modelfile"`
}

type OllamaCopyRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type OllamaPushRequest struct {
	Name string `json:"name"`
}

// Client interacts with a local Ollama instance.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{
			// The default client doesn't have a timeout here because we use contexts
		},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.HTTPClient.Do(req)
}

// GenerateStream sends an inference request and returns the streaming response body.
func (c *Client) GenerateStream(ctx context.Context, req models.InferenceRequest) (io.ReadCloser, int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/generate", req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, fmt.Errorf("ollama error: %s", string(respBody))
	}

	return resp.Body, resp.StatusCode, nil
}

// ChatStream sends a chat request and returns the streaming response body.
func (c *Client) ChatStream(ctx context.Context, req models.ChatRequest) (io.ReadCloser, int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/chat", req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, fmt.Errorf("ollama error: %s", string(respBody))
	}

	return resp.Body, resp.StatusCode, nil
}

// GetLoadedModels returns the list of models currently loaded.
func (c *Client) GetLoadedModels(ctx context.Context) ([]string, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/ps", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ps failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// ListLocalModels returns all models available on disk.
func (c *Client) ListLocalModels(ctx context.Context) ([]models.ModelInfo, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tags failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Models []models.ModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Models, nil
}

// Delete removes a model from disk.
func (c *Client) Delete(ctx context.Context, model string) error {
	resp, err := c.doRequest(ctx, "DELETE", "/api/delete", OllamaDeleteRequest{Name: model})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete failed: %d", resp.StatusCode)
	}
	return nil
}

// Pull triggers a model download in Ollama.
func (c *Client) Pull(ctx context.Context, model string) error {
	resp, err := c.doRequest(ctx, "POST", "/api/pull", OllamaPullRequest{Name: model})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull failed: %d", resp.StatusCode)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Unload unloads a model from memory.
func (c *Client) Unload(ctx context.Context, model string) error {
	req := OllamaUnloadRequest{
		Model:     model,
		Prompt:    "",
		KeepAlive: 0,
	}
	resp, err := c.doRequest(ctx, "POST", "/api/generate", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unload failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// Show returns metadata for a model.
func (c *Client) Show(ctx context.Context, model string) (map[string]interface{}, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/show", OllamaShowRequest{Name: model})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("show failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// Embeddings generates embeddings for a given input.
func (c *Client) Embeddings(ctx context.Context, model string, input interface{}) (io.ReadCloser, int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/embeddings", OllamaEmbeddingsRequest{Model: model, Input: input})
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, fmt.Errorf("embeddings failed: %s", string(respBody))
	}
	return resp.Body, resp.StatusCode, nil
}

// Create creates a model from a Modelfile.
func (c *Client) Create(ctx context.Context, name, modelfile string) (io.ReadCloser, int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/create", OllamaCreateRequest{Name: name, Modelfile: modelfile})
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, fmt.Errorf("create failed: %s", string(respBody))
	}
	return resp.Body, resp.StatusCode, nil
}

// Copy copies a model.
func (c *Client) Copy(ctx context.Context, source, destination string) (int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/copy", OllamaCopyRequest{Source: source, Destination: destination})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("copy failed: %s", string(respBody))
	}
	return resp.StatusCode, nil
}

// Push pushes a model to a registry.
func (c *Client) Push(ctx context.Context, name string) (io.ReadCloser, int, error) {
	resp, err := c.doRequest(ctx, "POST", "/api/push", OllamaPushRequest{Name: name})
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, fmt.Errorf("push failed: %s", string(respBody))
	}
	return resp.Body, resp.StatusCode, nil
}

// Version returns the Ollama version.
func (c *Client) Version(ctx context.Context) (string, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/version", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("version failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		Version string `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Version, nil
}
