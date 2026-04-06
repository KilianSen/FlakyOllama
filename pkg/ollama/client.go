package ollama

import (
	"FlakyOllama/pkg/models"
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

// Generate sends an inference request to Ollama.
func (c *Client) Generate(req models.InferenceRequest) (models.InferenceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return models.InferenceResponse{}, err
	}

	resp, err := http.Post(c.BaseURL+"/api/generate", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return models.InferenceResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return models.InferenceResponse{}, fmt.Errorf("ollama error: %s", string(respBody))
	}

	// For simplicity, we assume non-streaming for now, but in reality we'd handle both.
	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return models.InferenceResponse{}, err
	}

	return models.InferenceResponse{Response: result.Response}, nil
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
	return nil
}

// Unload unloads a model from memory.
func (c *Client) Unload(model string) error {
	// To unload, we send a generate request with keep_alive: 0
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
