// internal/models/metadata.go
package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k0kubun/pp"
)

// ModelMeta holds normalized metadata for a model across hosts.
type ModelMeta struct {
	Type     string         `json:"type,omitempty"` // either "llama.cpp" or "ollama"
	Name     string         `json:"name,omitempty"`
	Endpoint string         `json:"endpoint,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// FetchEndpointModelNames queries each URL for known model endpoints and returns unique model names.
func FetchEndpointModelNames(urls []string) []ModelMeta {
	var models []ModelMeta
	client := &http.Client{Timeout: 30 * time.Second}

	for _, rawURL := range urls {
		baseURL := strings.TrimRight(strings.TrimSpace(rawURL), "/")
		if baseURL == "" {
			continue
		}

		if found := fetchLlamaModels(client, baseURL, &models); found {
			continue
		}

		if found := fetchOllamaModels(client, baseURL, &models); found {
			continue
		}

		fmt.Printf("Endpoint is neither \"llama.cpp\" or \"ollama\": %s\n", baseURL)
		models = append(models, ModelMeta{Name: baseURL, Endpoint: baseURL})
	}

	return uniqueModelMeta(models)
}

// FetchModelMetadata queries each model for detailed metadata and prints results.
func FetchModelMetadata(models []ModelMeta) {
	client := &http.Client{Timeout: 30 * time.Second}

	for i := range models {
		if metadataExists(models[i]) {
			continue
		}

		fmt.Println("Fetching Model Metadata for: ", models[i].Type, models[i].Endpoint, models[i].Name)
		switch models[i].Type {
		case "llama.cpp":
			fetchLlamaModelMetadata(client, &models[i])
		case "ollama":
			fetchOllamaModelMetadata(client, &models[i])
		default:
			fmt.Printf("Unknown model type for %s: %s\n", models[i].Name, models[i].Type)
		}
	}

	pp.Println(models)
}

func fetchLlamaModels(client *http.Client, baseURL string, models *[]ModelMeta) bool {
	resp, err := client.Get(baseURL + "/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading /models response from %s: %v\n", baseURL, err)
		return true
	}

	llamaModels, err := parseLlamaModels(body)
	if err != nil {
		fmt.Printf("Error parsing /models response from %s: %v\n", baseURL, err)
		return true
	}

	for _, model := range llamaModels {
		if name := modelDisplayName(model); name != "" {
			*models = append(*models, ModelMeta{
				Type:     "llama.cpp",
				Name:     name,
				Endpoint: baseURL,
			})
		}
	}

	return true
}

func fetchOllamaModels(client *http.Client, baseURL string, models *[]ModelMeta) bool {
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading /api/tags response from %s: %v\n", baseURL, err)
		return true
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &tagsResp); err != nil {
		fmt.Printf("Error parsing /api/tags response from %s: %v\n", baseURL, err)
		return true
	}

	for _, model := range tagsResp.Models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		*models = append(*models, ModelMeta{
			Type:     "ollama",
			Name:     name,
			Endpoint: baseURL,
		})
	}

	return true
}

func fetchLlamaModelMetadata(client *http.Client, model *ModelMeta) {
	if model.Endpoint == "" || model.Name == "" {
		return
	}

	wasLoaded, err := isLlamaModelLoaded(client, model)
	if err != nil {
		fmt.Printf("Error checking load state for %s: %v\n", model.Name, err)
	}
	if !wasLoaded {
		if err := loadLlamaModel(client, model); err != nil {
			fmt.Printf("Error loading model %s on %s: %v\n", model.Name, model.Endpoint, err)
			return
		}
		if err := waitForLlamaModelLoaded(client, model, 30*time.Second); err != nil {
			fmt.Printf("Error waiting for model %s to load on %s: %v\n", model.Name, model.Endpoint, err)
			return
		}
	}

	reqURL := fmt.Sprintf("%s/props?model=%s", strings.TrimRight(model.Endpoint, "/"), url.QueryEscape(model.Name))
	resp, err := client.Get(reqURL)
	if err != nil {
		fmt.Printf("Error fetching /props for %s: %v\n", model.Name, err)
		if !wasLoaded {
			unloadLlamaModel(client, model)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Non-OK /props response for %s: %s\n", model.Name, resp.Status)
		if !wasLoaded {
			unloadLlamaModel(client, model)
		}
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading /props response for %s: %v\n", model.Name, err)
		if !wasLoaded {
			unloadLlamaModel(client, model)
		}
		return
	}

	updateModelMetadataFromJSON(body, model, "/props")

	if model.Metadata != nil {
		WriteModelMetadata([]ModelMeta{*model})
	}

	if !wasLoaded {
		unloadLlamaModel(client, model)
	}
}

func fetchOllamaModelMetadata(client *http.Client, model *ModelMeta) {
	if model.Endpoint == "" || model.Name == "" {
		return
	}

	payload, err := json.Marshal(map[string]string{"model": model.Name})
	if err != nil {
		fmt.Printf("Error encoding /api/show request for %s: %v\n", model.Name, err)
		return
	}

	reqURL := strings.TrimRight(model.Endpoint, "/") + "/api/show"
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("Error creating /api/show request for %s: %v\n", model.Name, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching /api/show for %s: %v\n", model.Name, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Non-OK /api/show response for %s: %s\n", model.Name, strings.TrimSpace(string(body)))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading /api/show response for %s: %v\n", model.Name, err)
		return
	}

	updateModelMetadataFromJSON(body, model, "/api/show")

	if model.Metadata != nil {
		WriteModelMetadata([]ModelMeta{*model})
	}
}

func updateModelMetadataFromJSON(body []byte, model *ModelMeta, endpoint string) {
	var metadata map[string]any
	if err := json.Unmarshal(body, &metadata); err != nil {
		fmt.Printf("Error parsing %s response for %s: %v\n", endpoint, model.Name, err)
		return
	}

	removeTensorsMetadata(metadata)
	model.Metadata = metadata
}

// WriteModelMetadata writes each model's metadata payload to disk.
func WriteModelMetadata(models []ModelMeta) {
	for _, model := range models {
		if model.Metadata != nil {
			removeTensorsMetadata(model.Metadata)
		}
		path := modelMetadataPath(model)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Printf("Error creating metadata directory for %s: %v\n", model.Name, err)
			continue
		}
		payload, err := json.MarshalIndent(model, "", "  ")
		if err != nil {
			fmt.Printf("Error encoding metadata for %s: %v\n", model.Name, err)
			continue
		}
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			fmt.Printf("Error writing metadata for %s: %v\n", model.Name, err)
			continue
		}
		fmt.Printf("Write:  %s\n", path)
	}
}

func removeTensorsMetadata(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "tensors")
		for _, entry := range typed {
			removeTensorsMetadata(entry)
		}
	case []any:
		for _, entry := range typed {
			removeTensorsMetadata(entry)
		}
	}
}

func loadLlamaModel(client *http.Client, model *ModelMeta) error {
	payload, err := json.Marshal(map[string]string{"model": model.Name})
	if err != nil {
		return err
	}

	reqURL := strings.TrimRight(model.Endpoint, "/") + "/models/load"
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest && !isAlreadyLoadedResponse(respBody) {
		return fmt.Errorf("llama.cpp: /models/load returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func unloadLlamaModel(client *http.Client, model *ModelMeta) {
	payload, err := json.Marshal(map[string]string{"model": model.Name})
	if err != nil {
		fmt.Printf("Error encoding unload payload for %s: %v\n", model.Name, err)
		return
	}

	reqURL := strings.TrimRight(model.Endpoint, "/") + "/models/unload"
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("Error creating unload request for %s: %v\n", model.Name, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error unloading model %s: %v\n", model.Name, err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		fmt.Printf("Error unloading model %s: %s\n", model.Name, strings.TrimSpace(string(respBody)))
	}
}

func isAlreadyLoadedResponse(body []byte) bool {
	text := strings.ToLower(strings.TrimSpace(string(body)))
	return strings.Contains(text, "already loaded")
}

func isLlamaModelLoaded(client *http.Client, model *ModelMeta) (bool, error) {
	reqURL := strings.TrimRight(model.Endpoint, "/") + "/models"
	resp, err := client.Get(reqURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("non-OK /models response: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	llamaModels, err := parseLlamaModels(body)
	if err != nil {
		return false, err
	}

	for _, item := range llamaModels {
		if strings.EqualFold(modelDisplayName(item), model.Name) {
			return strings.EqualFold(modelStatusValue(item), "loaded"), nil
		}
	}

	return false, nil
}

func waitForLlamaModelLoaded(client *http.Client, model *ModelMeta, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		loaded, err := isLlamaModelLoaded(client, model)
		if err == nil && loaded {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("timeout waiting for model to load")
		}
		<-ticker.C
	}
}

func uniqueModelMeta(models []ModelMeta) []ModelMeta {
	seen := make(map[string]struct{})
	unique := make([]ModelMeta, 0, len(models))

	for _, model := range models {
		key := strings.Join([]string{model.Type, model.Name, model.Endpoint}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, model)
	}

	return unique
}

func metadataExists(model ModelMeta) bool {
	path := modelMetadataPath(model)
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Error checking metadata file for %s: %v\n", model.Name, err)
	}
	return false
}

func modelMetadataPath(model ModelMeta) string {
	name := sanitizeModelFilename(model.Name)
	filename := fmt.Sprintf("%s_%s.json", model.Type, name)
	return filepath.Join("agonData", "modelMetadata", filename)
}

func sanitizeModelFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	return replacer.Replace(name)
}

func parseLlamaModels(body []byte) ([]llamaModel, error) {
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

	return nil, fmt.Errorf("unrecognized /models response")
}
