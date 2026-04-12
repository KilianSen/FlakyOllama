package ollama

import (
	"FlakyOllama/pkg/shared/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client interacts with a local Ollama instance.
type Client struct {
	BaseURL string
}

func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL}
}

// GenerateStream sends an inference request and returns the streaming response body.
func (c *Client) GenerateStream(req models.InferenceRequest) (io.ReadCloser, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	resp, err := http.Post(c.BaseURL+"/api/generate", "application/json", bytes.NewBuffer(body))
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
func (c *Client) ChatStream(req models.ChatRequest) (io.ReadCloser, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	resp, err := http.Post(c.BaseURL+"/api/chat", "application/json", bytes.NewBuffer(body))
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
func (c *Client) GetLoadedModels() ([]string, error) {
	// Ollama /api/ps endpoint returns running models.
	resp, err := http.Get(c.BaseURL + "/api/ps")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
func (c *Client) ListLocalModels() ([]models.ModelInfo, error) {
	resp, err := http.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []models.ModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Models, nil
}

// Delete removes a model from disk.
func (c *Client) Delete(model string) error {
	req, _ := http.NewRequest("DELETE", c.BaseURL+"/api/delete", bytes.NewBuffer([]byte(fmt.Sprintf(`{"name":"%s"}`, model))))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
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
func (c *Client) Pull(model string) error {
	body, _ := json.Marshal(map[string]string{"name": model})
	resp, err := http.Post(c.BaseURL+"/api/pull", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull failed: %d", resp.StatusCode)
	}

	// Ollama /api/pull is a stream. We must consume it to let the pull finish.
	// We'll discard the status updates for now to keep things simple.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Unload unloads a model from memory.
func (c *Client) Unload(model string) error {
	req := map[string]interface{}{
		"model":      model,
		"prompt":     "",
		"keep_alive": 0,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(c.BaseURL+"/api/generate", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Show returns metadata for a model.
func (c *Client) Show(model string) (map[string]interface{}, error) {
	body, _ := json.Marshal(map[string]string{"name": model})
	resp, err := http.Post(c.BaseURL+"/api/show", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// Embeddings generates embeddings for a given input.
func (c *Client) Embeddings(model string, input interface{}) (io.ReadCloser, int, error) {
	req := map[string]interface{}{
		"model": model,
		"input": input,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(c.BaseURL+"/api/embeddings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.StatusCode, nil
}

// Create creates a model from a Modelfile.
func (c *Client) Create(name, modelfile string) (io.ReadCloser, int, error) {
	req := map[string]interface{}{
		"name":      name,
		"modelfile": modelfile,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(c.BaseURL+"/api/create", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.StatusCode, nil
}

// Copy copies a model.
func (c *Client) Copy(source, destination string) (int, error) {
	req := map[string]interface{}{
		"source":      source,
		"destination": destination,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(c.BaseURL+"/api/copy", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// Push pushes a model to a registry.
func (c *Client) Push(name string) (io.ReadCloser, int, error) {
	req := map[string]interface{}{
		"name": name,
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(c.BaseURL+"/api/push", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.StatusCode, nil
}

// Version returns the Ollama version.
func (c *Client) Version() (string, error) {
	resp, err := http.Get(c.BaseURL + "/api/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Version string `json:"version"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Version, nil
}
