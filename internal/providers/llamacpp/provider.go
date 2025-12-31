// internal/providers/llamacpp/provider.go
// Package llamacpp provides a ChatProvider backed by llama.cpp's OpenAI-compatible HTTP API.
package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
)

// Provider implements the providers.ChatProvider interface using llama.cpp HTTP APIs.
type Provider struct {
	client  *http.Client
	timeout time.Duration
	debug   bool
}

// New constructs a Provider configured with the application's request timeout.
func New(cfg *appconfig.Config) *Provider {
	timeout := cfg.RequestTimeout()
	return &Provider{
		client: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{ForceAttemptHTTP2: false},
		},
		timeout: timeout,
		debug:   cfg.Debug,
	}
}

type modelsResponse struct {
	Data   []llamaModel `json:"data"`
	Models []llamaModel `json:"models"`
}

type llamaModel struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Model  string      `json:"model"`
	Path   string      `json:"path"`
	Status statusField `json:"status"`
}

// LoadedModels returns the models currently loaded in memory on the host.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	models, err := p.fetchModels(ctx, host, true)
	if err != nil {
		return nil, err
	}

	var loaded []string
	for _, model := range models {
		status := strings.TrimSpace(modelStatusValue(model))
		if strings.EqualFold(status, "loaded") {
			name := modelDisplayName(model)
			if name != "" {
				loaded = append(loaded, name)
			}
		}
	}
	return loaded, nil
}

// EnsureModelReady triggers a load request when the router endpoints are available.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	payload := map[string]any{"model": model}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	endpoint := host.URL + "/models/load"
	logging.LogRequest("AGON->LLM", hostIdentifier(host), model, "", body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logging.LogRequest("LLM->AGON", hostIdentifier(host), model, "", respBody)

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		// Router endpoints not available; rely on auto-loading on first request.
		return nil
	}
	if resp.StatusCode >= 400 {
		if isAlreadyLoadedError(resp.StatusCode, respBody) {
			return p.waitForModelLoaded(ctx, host, model)
		}
		return fmt.Errorf("llama.cpp: /models/load returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return p.waitForModelLoaded(ctx, host, model)
}

// Stream issues a chat request and forwards output to the provided callbacks.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	messages := req.History
	if req.SystemPrompt != "" {
		messages = append([]providers.ChatMessage{{Role: "system", Content: req.SystemPrompt}}, messages...)
	}
	if len(messages) == 0 {
		messages = []providers.ChatMessage{}
	}
	messages = sanitizeMessages(messages)
	openAIMessages := toOpenAIMessages(messages)

	if strings.TrimSpace(req.Model) != "" {
		if err := p.EnsureModelReady(ctx, req.Host, req.Model); err != nil {
			return err
		}
	}

	payload := map[string]any{
		"model":    req.Model,
		"messages": openAIMessages,
		"stream":   !req.DisableStreaming,
	}
	applyParameters(payload, req.Parameters)
	if req.JSONMode {
		payload["response_format"] = map[string]any{"type": "json_object"}
	}
	if len(req.Tools) > 0 && p.debug {
		logging.LogEvent("llama.cpp: tools are not forwarded yet; count=%d", len(req.Tools))
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	logging.LogRequest("AGON->LLM", hostIdentifier(req.Host), req.Model, "", body)

	streamCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	endpoint := req.Host.URL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if !req.DisableStreaming {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		logging.LogRequest("LLM->AGON", hostIdentifier(req.Host), req.Model, "", raw)
		return fmt.Errorf("llama.cpp: /v1/chat/completions returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	if req.DisableStreaming {
		return p.handleNonStreaming(resp, req, callbacks)
	}
	return p.handleStreaming(resp, req, callbacks)
}

func (p *Provider) handleNonStreaming(resp *http.Response, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logging.LogRequest("LLM->AGON", hostIdentifier(req.Host), req.Model, "", body)

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("llama.cpp: chat response contained no choices")
	}

	content := parsed.Choices[0].Message.Content
	role := parsed.Choices[0].Message.Role
	if role == "" {
		role = "assistant"
	}
	if callbacks.OnChunk != nil && strings.TrimSpace(content) != "" {
		if err := callbacks.OnChunk(providers.ChatMessage{Role: role, Content: content}); err != nil {
			return err
		}
	}
	if callbacks.OnComplete != nil {
		modelName := parsed.Model
		if modelName == "" {
			modelName = req.Model
		}
		meta := providers.StreamMetadata{
			Model:     modelName,
			CreatedAt: time.Now(),
			Done:      true,
		}
		if err := callbacks.OnComplete(meta); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) handleStreaming(resp *http.Response, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	reader := bufio.NewReader(resp.Body)
	var finalModel string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		logging.LogRequest("LLM->AGON", hostIdentifier(req.Host), req.Model, "", data)

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if chunk.Model != "" {
			finalModel = chunk.Model
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		content := choice.Delta.Content
		role := choice.Delta.Role
		if content == "" && choice.Message.Content != "" {
			content = choice.Message.Content
			role = choice.Message.Role
		}
		if role == "" {
			role = "assistant"
		}
		if callbacks.OnChunk != nil && strings.TrimSpace(content) != "" {
			if err := callbacks.OnChunk(providers.ChatMessage{Role: role, Content: content}); err != nil {
				return err
			}
		}
	}

	if callbacks.OnComplete != nil {
		modelName := finalModel
		if modelName == "" {
			modelName = req.Model
		}
		meta := providers.StreamMetadata{
			Model:     modelName,
			CreatedAt: time.Now(),
			Done:      true,
		}
		if err := callbacks.OnComplete(meta); err != nil {
			return err
		}
	}
	return nil
}

// Close releases any resources held by the provider.
func (p *Provider) Close() error {
	return nil
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type chatStreamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func parseModels(body []byte) ([]llamaModel, error) {
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

	return nil, fmt.Errorf("llama.cpp: unrecognized /models response")
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

func (p *Provider) fetchModels(ctx context.Context, host appconfig.Host, logIO bool) ([]llamaModel, error) {
	endpoint := host.URL + "/models"
	if logIO {
		logging.LogRequest("AGON->LLM", hostIdentifier(host), "", "", map[string]string{"method": http.MethodGet, "url": endpoint})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if logIO {
		logging.LogRequest("LLM->AGON", hostIdentifier(host), "", "", body)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp: /models returned %s", resp.Status)
	}

	return parseModels(body)
}

func (p *Provider) waitForModelLoaded(ctx context.Context, host appconfig.Host, model string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		loaded, err := p.isModelLoaded(ctx, host, model)
		if err != nil {
			return err
		}
		if loaded {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("llama.cpp: model %s did not load before timeout", model)
		case <-ticker.C:
		}
	}
}

func (p *Provider) isModelLoaded(ctx context.Context, host appconfig.Host, model string) (bool, error) {
	models, err := p.fetchModels(ctx, host, false)
	if err != nil {
		return false, err
	}
	for _, item := range models {
		if strings.EqualFold(modelDisplayName(item), model) {
			status := strings.ToLower(modelStatusValue(item))
			return status == "loaded", nil
		}
	}
	return false, nil
}

func isAlreadyLoadedError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(string(body)))
	if strings.Contains(text, "already loaded") {
		return true
	}
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if strings.Contains(strings.ToLower(payload.Error.Message), "already loaded") {
			return true
		}
	}
	return false
}

func applyParameters(payload map[string]any, params appconfig.Parameters) {
	if params.TopK != nil {
		payload["top_k"] = *params.TopK
	}
	if params.TopP != nil {
		payload["top_p"] = *params.TopP
	}
	if params.MinP != nil {
		payload["min_p"] = *params.MinP
	}
	if params.TFSZ != nil {
		payload["tfs_z"] = *params.TFSZ
	}
	if params.TypicalP != nil {
		payload["typical_p"] = *params.TypicalP
	}
	if params.RepeatLastN != nil {
		payload["repeat_last_n"] = *params.RepeatLastN
	}
	if params.Temperature != nil {
		payload["temperature"] = *params.Temperature
	}
	if params.RepeatPenalty != nil {
		payload["repeat_penalty"] = *params.RepeatPenalty
	}
	if params.PresencePenalty != nil {
		payload["presence_penalty"] = *params.PresencePenalty
	}
	if params.FrequencyPenalty != nil {
		payload["frequency_penalty"] = *params.FrequencyPenalty
	}
}

func sanitizeMessages(messages []providers.ChatMessage) []providers.ChatMessage {
	if len(messages) == 0 {
		return messages
	}
	sanitized := make([]providers.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" {
			role = "user"
		}
		if role != "assistant" && content == "" {
			continue
		}
		sanitized = append(sanitized, providers.ChatMessage{Role: role, Content: content})
	}
	if len(sanitized) == 0 {
		return []providers.ChatMessage{}
	}
	return sanitized
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func toOpenAIMessages(messages []providers.ChatMessage) []openAIMessage {
	if len(messages) == 0 {
		return []openAIMessage{}
	}
	out := make([]openAIMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, openAIMessage{
			Role:    strings.TrimSpace(msg.Role),
			Content: strings.TrimSpace(msg.Content),
		})
	}
	return out
}

// hostIdentifier returns a string identifier for a given host, preferring the name over the URL.
func hostIdentifier(host appconfig.Host) string {
	name := strings.TrimSpace(host.Name)
	if name != "" {
		return name
	}
	if url := strings.TrimSpace(host.URL); url != "" {
		return url
	}
	return "llama.cpp-host"
}
