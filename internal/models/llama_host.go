// internal/models/llama_host.go
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
	"github.com/mwiater/agon/internal/logging"
)

// LlamaCppHost implements the LLMHost interface for llama.cpp servers.
type LlamaCppHost struct {
	Name           string
	URL            string
	Models         []string
	client         *http.Client
	requestTimeout time.Duration
}

// GetName returns the display name of the llama.cpp host.
func (h *LlamaCppHost) GetName() string {
	return h.Name
}

// GetType returns the type identifier for llama.cpp hosts ("llama.cpp").
func (h *LlamaCppHost) GetType() string {
	return "llama.cpp"
}

// GetModels returns the configured models for the llama.cpp host.
func (h *LlamaCppHost) GetModels() []string {
	return h.Models
}

// httpClient returns the explicitly configured HTTP client or the shared default client.
func (h *LlamaCppHost) httpClient() *http.Client {
	if h.client != nil {
		return h.client
	}
	return http.DefaultClient
}

// effectiveTimeout resolves the timeout to use for outbound HTTP requests.
func (h *LlamaCppHost) effectiveTimeout() time.Duration {
	return h.requestTimeout
}

// doRequest executes an HTTP request against the llama.cpp API with context cancellation support.
func (h *LlamaCppHost) doRequest(method, path string, body io.Reader, contentType string) (*http.Response, context.CancelFunc, error) {
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

// PullModel is not supported for llama.cpp hosts.
func (h *LlamaCppHost) PullModel(model string) {
	fmt.Printf("Pulling models is not supported for %s (%s)\n", h.Name, h.GetType())
}

// DeleteModel is not supported for llama.cpp hosts.
func (h *LlamaCppHost) DeleteModel(model string) {
	fmt.Printf("Deleting models is not supported for %s (%s)\n", h.Name, h.GetType())
}

// UnloadModel unloads a model from a llama.cpp host using the /models/unload endpoint.
func (h *LlamaCppHost) UnloadModel(model string) {
	payload := map[string]string{"model": model}
	body, _ := json.Marshal(payload)

	logging.LogRequest("AGON->LLM", hostIdentifier(h), model, "", body)
	resp, cancel, err := h.doRequest(http.MethodPost, "/models/unload", bytes.NewReader(body), "application/json")
	if err != nil {
		fmt.Printf("Error unloading model %s on %s: %v\n", model, h.Name, err)
		return
	}
	defer cancel()
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	logging.LogRequest("LLM->AGON", hostIdentifier(h), model, "", respBody)
	if resp.StatusCode >= 400 {
		fmt.Printf("Error unloading model %s on %s: %s\n", model, h.Name, strings.TrimSpace(string(respBody)))
	}
}

// ListRawModels returns the models available on a llama.cpp host without styling markup.
func (h *LlamaCppHost) ListRawModels() ([]string, error) {
	models, err := h.listModels()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, model := range models {
		if name := modelDisplayName(model); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ListModels returns the models available on a llama.cpp host, labeling their status.
func (h *LlamaCppHost) ListModels() ([]string, error) {
	loadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	loadingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	unloadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	models, err := h.listModels()
	if err != nil {
		return nil, err
	}

	var formatted []string
	for _, model := range models {
		name := modelDisplayName(model)
		if name == "" {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(modelStatusValue(model)))
		if status == "" {
			status = "UNKNOWN"
		}
		entry := fmt.Sprintf("- %s (%s)", name, status)
		switch strings.ToLower(status) {
		case "loaded":
			formatted = append(formatted, loadedStyle.Render(entry))
		case "loading":
			formatted = append(formatted, loadingStyle.Render(entry))
		default:
			formatted = append(formatted, unloadedStyle.Render(entry))
		}
	}
	return formatted, nil
}

// GetRunningModels returns the set of currently loaded models on a llama.cpp host.
func (h *LlamaCppHost) GetRunningModels() (map[string]struct{}, error) {
	models, err := h.listModels()
	if err != nil {
		return nil, err
	}
	running := make(map[string]struct{})
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(modelStatusValue(model)), "loaded") {
			if name := modelDisplayName(model); name != "" {
				running[name] = struct{}{}
			}
		}
	}
	return running, nil
}

// GetModelParameters is not supported for llama.cpp hosts.
func (h *LlamaCppHost) GetModelParameters() ([]ModelParameters, error) {
	return nil, fmt.Errorf("model parameters are not supported for %s (%s)", h.Name, h.GetType())
}

type llamaModel struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Model  string      `json:"model"`
	Path   string      `json:"path"`
	Status statusField `json:"status"`
}

type modelsResponse struct {
	Data   []llamaModel `json:"data"`
	Models []llamaModel `json:"models"`
}

func (h *LlamaCppHost) listModels() ([]llamaModel, error) {
	logging.LogRequest("AGON->LLM", hostIdentifier(h), "", "", map[string]string{
		"method": http.MethodGet,
		"url":    h.URL + "/models",
	})
	resp, cancel, err := h.doRequest(http.MethodGet, "/models", nil, "")
	if err != nil {
		return nil, fmt.Errorf("could not list models: llama.cpp is not accessible on %s", h.Name)
	}
	defer cancel()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body from %s: %v", h.Name, err)
	}
	logging.LogRequest("LLM->AGON", hostIdentifier(h), "", "", body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not list models: %s", strings.TrimSpace(string(body)))
	}

	var wrapped modelsResponse
	if err := json.Unmarshal(body, &wrapped); err == nil {
		if len(wrapped.Models) > 0 {
			return wrapped.Models, nil
		}
		if len(wrapped.Data) > 0 {
			return wrapped.Data, nil
		}
	}

	var direct []llamaModel
	if err := json.Unmarshal(body, &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}

	var names struct {
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(body, &names); err == nil && len(names.Models) > 0 {
		out := make([]llamaModel, 0, len(names.Models))
		for _, name := range names.Models {
			out = append(out, llamaModel{Name: name})
		}
		return out, nil
	}

	return nil, fmt.Errorf("unrecognized /models response from %s", h.Name)
}

func modelDisplayName(model llamaModel) string {
	if strings.TrimSpace(model.ID) != "" {
		return strings.TrimSpace(model.ID)
	}
	if strings.TrimSpace(model.Name) != "" {
		return strings.TrimSpace(model.Name)
	}
	if strings.TrimSpace(model.Model) != "" {
		return strings.TrimSpace(model.Model)
	}
	return strings.TrimSpace(model.Path)
}

type statusField struct {
	Value string
}

func (s *statusField) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		s.Value = ""
		return nil
	}
	if trimmed[0] == '"' {
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		s.Value = v
		return nil
	}
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	s.Value = obj.Value
	return nil
}

func modelStatusValue(model llamaModel) string {
	return strings.TrimSpace(model.Status.Value)
}

func hostIdentifier(host *LlamaCppHost) string {
	name := strings.TrimSpace(host.Name)
	if name != "" {
		return name
	}
	if url := strings.TrimSpace(host.URL); url != "" {
		return url
	}
	return "llama.cpp-host"
}
