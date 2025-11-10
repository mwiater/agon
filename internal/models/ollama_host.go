// internal/models/ollama_host.go
package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// OllamaHost implements the LLMHost interface for Ollama servers.
type OllamaHost struct {
	Name           string
	URL            string
	Models         []string
	client         *http.Client
	requestTimeout time.Duration
}

// GetName returns the display name of the Ollama host.
func (h *OllamaHost) GetName() string {
	return h.Name
}

// GetType returns the type identifier for Ollama hosts ("ollama").
func (h *OllamaHost) GetType() string {
	return "ollama"
}

// GetModels returns the configured models for the Ollama host.
func (h *OllamaHost) GetModels() []string {
	return h.Models
}

// httpClient returns the explicitly configured HTTP client or the shared default client.
func (h *OllamaHost) httpClient() *http.Client {
	if h.client != nil {
		return h.client
	}
	return http.DefaultClient
}

// effectiveTimeout resolves the timeout to use for outbound HTTP requests.
func (h *OllamaHost) effectiveTimeout() time.Duration {
	return h.requestTimeout
}

// doRequest executes an HTTP request against the Ollama API with context cancellation support.
func (h *OllamaHost) doRequest(method, path string, body io.Reader, contentType string) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), h.effectiveTimeout())
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("%s%s", h.URL, path), body)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := h.httpClient().Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return resp, cancel, nil
}

// PullModel pulls the provided model to the Ollama host via the /api/pull endpoint.
func (h *OllamaHost) PullModel(model string) {
	payload := map[string]string{"name": model}
	body, _ := json.Marshal(payload)

	resp, cancel, err := h.doRequest(http.MethodPost, "/api/pull", bytes.NewReader(body), "application/json")
	if err != nil {
		fmt.Printf("Error pulling model %s on %s: %v\n", model, h.Name, err)
		return
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error pulling model %s on %s: %s\n", model, h.Name, strings.TrimSpace(string(respBody)))
	}
}

// DeleteModel deletes the specified model from an Ollama host via the /api/delete endpoint.
func (h *OllamaHost) DeleteModel(model string) {
	payload := map[string]string{"model": model}
	body, _ := json.Marshal(payload)

	resp, cancel, err := h.doRequest(http.MethodDelete, "/api/delete", bytes.NewReader(body), "application/json")
	if err != nil {
		fmt.Printf("Error deleting model %s on %s: %v\n", model, h.Name, err)
		return
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error deleting model %s on %s: %s\n", model, h.Name, strings.TrimSpace(string(respBody)))
	}
}

// UnloadModel unloads a model from an Ollama host by sending a chat request with keep_alive set to 0.
func (h *OllamaHost) UnloadModel(model string) {
	payload := map[string]any{"model": model, "keep_alive": 0}
	body, _ := json.Marshal(payload)

	resp, cancel, err := h.doRequest(http.MethodPost, "/api/chat", bytes.NewReader(body), "application/json")
	if err != nil {
		fmt.Printf("Error unloading model %s on %s: %v\n", model, h.Name, err)
		return
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error unloading model %s on %s: %s\n", model, h.Name, strings.TrimSpace(string(respBody)))
	}
}

// ListRawModels returns the models available on an Ollama host without styling markup.
func (h *OllamaHost) ListRawModels() ([]string, error) {
	resp, cancel, err := h.doRequest(http.MethodGet, "/api/tags", nil, "")
	if err != nil {
		return nil, fmt.Errorf("could not list models: Ollama is not accessible on %s", h.Name)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("could not list models: %s", strings.TrimSpace(string(bodyBytes)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body from %s: %v", h.Name, err)
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("error parsing models from %s: %v", h.Name, err)
	}

	var models []string
	for _, model := range tagsResp.Models {
		models = append(models, model.Name)
	}
	return models, nil
}

// ListModels returns the models available on an Ollama host, labeling currently loaded entries.
func (h *OllamaHost) ListModels() ([]string, error) {
	modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	loadedModelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))

	runningModels, err := h.GetRunningModels()
	if err != nil {
		return nil, fmt.Errorf("could not get running models: %v", err)
	}

	resp, cancel, err := h.doRequest(http.MethodGet, "/api/tags", nil, "")
	if err != nil {
		return nil, fmt.Errorf("could not list models: Ollama is not accessible on %s", h.Name)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("could not list models: %s", strings.TrimSpace(string(bodyBytes)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body from %s: %v", h.Name, err)
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("error parsing models from %s: %v", h.Name, err)
	}

	var models []string
	for _, model := range tagsResp.Models {
		if _, ok := runningModels[model.Name]; ok {
			models = append(models, loadedModelStyle.Render(fmt.Sprintf("- %s (CURRENTLY LOADED)", model.Name)))
		} else {
			models = append(models, modelStyle.Render(fmt.Sprintf("- %s", model.Name)))
		}
	}
	return models, nil
}

// GetRunningModels returns the set of currently running models on an Ollama host by querying /api/ps.
func (h *OllamaHost) GetRunningModels() (map[string]struct{}, error) {
	runningModels := make(map[string]struct{})

	resp, cancel, err := h.doRequest(http.MethodGet, "/api/ps", nil, "")
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("could not get running models: %s", strings.TrimSpace(string(bodyBytes)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var psResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &psResp); err != nil {
		return nil, err
	}

	for _, model := range psResp.Models {
		runningModels[model.Name] = struct{}{}
	}

	return runningModels, nil
}

// GetModelParameters retrieves the parameters for each model on the host.
func (h *OllamaHost) GetModelParameters() ([]ModelParameters, error) {
	resp, cancel, err := h.doRequest(http.MethodGet, "/api/tags", nil, "")
	if err != nil {
		return nil, fmt.Errorf("could not list models: Ollama is not accessible on %s", h.Name)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("could not list models: %s", strings.TrimSpace(string(bodyBytes)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body from %s: %v", h.Name, err)
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		return nil, fmt.Errorf("error parsing models from %s: %v", h.Name, err)
	}

	var allParams []ModelParameters
	for _, m := range tagsResp.Models {
		params, err := h.getModelParametersFromAPI(m.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting parameters for model %s from %s: %v", m.Name, h.Name, err)
		}
		allParams = append(allParams, params)
	}

	return allParams, nil
}

// getModelParametersFromAPI retrieves the parameters for a single model from the API.
func (h *OllamaHost) getModelParametersFromAPI(model string) (ModelParameters, error) {
	payload := map[string]string{"name": model}
	body, _ := json.Marshal(payload)

	resp, cancel, err := h.doRequest(http.MethodPost, "/api/show", bytes.NewReader(body), "application/json")
	if err != nil {
		return ModelParameters{}, fmt.Errorf("error getting model parameters for %s on %s: %v", model, h.Name, err)
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return ModelParameters{}, fmt.Errorf("error getting model parameters for %s on %s: %s", model, h.Name, strings.TrimSpace(string(respBody)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ModelParameters{}, fmt.Errorf("error reading response body from %s: %v", h.Name, err)
	}

	var params ModelParameters
	if err := json.Unmarshal(respBody, &params); err != nil {
		return ModelParameters{}, fmt.Errorf("error parsing model parameters from %s: %v", h.Name, err)
	}

	params.Model = model

	return params, nil
}

// extractSettings parses the modelfile parameters text and returns the sampling settings relevant to Agon.
func extractSettings(paramsText string) map[string]string {
	wanted := map[string]string{
		"temperature":    "n/a",
		"top_p":          "n/a",
		"top_k":          "n/a",
		"repeat_penalty": "n/a",
		"min_p":          "n/a",
	}

	lines := strings.Split(paramsText, "\n")
	for _, line := range lines {
		s := strings.TrimSpace(strings.ToLower(line))
		if s == "" {
			continue
		}

		if strings.Contains(s, "=") {
			kv := strings.SplitN(s, "=", 2)
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			if _, ok := wanted[key]; ok && wanted[key] == "n/a" && val != "" {
				wanted[key] = val
			}
			continue
		}

		fields := strings.Fields(s)
		if len(fields) >= 2 {
			key := fields[0]
			valIdx := 1
			if key == "parameter" && len(fields) >= 3 {
				key = fields[1]
				valIdx = 2
			}
			if _, ok := wanted[key]; ok && wanted[key] == "n/a" {
				wanted[key] = fields[valIdx]
			}
		}
	}

	return wanted
}