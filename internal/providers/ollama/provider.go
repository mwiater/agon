// Package ollama provides a ChatProvider backed by Ollama-compatible HTTP endpoints.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

// Provider implements the providers.ChatProvider interface using Ollama HTTP APIs.
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

type ollamaPsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (p *Provider) logTools(tools []providers.ToolDefinition) {
	if !p.debug {
		return
	}
	if len(tools) == 0 {
		log.Printf("Tools: false")
		return
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Name != "" {
			names = append(names, tool.Name)
		}
	}
	if len(names) == 0 {
		log.Printf("Tools: false")
		return
	}
	log.Printf("Tools: {%s}", strings.Join(names, ", "))
}

type toolCall struct {
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func normalizeToolArgs(toolName string, args map[string]any, availableTools []providers.ToolDefinition) map[string]any {
	normalized := make(map[string]any, len(args))
	for k, v := range args {
		normalized[k] = v
	}
	if toolName == "" && len(availableTools) == 1 {
		toolName = availableTools[0].Name
	}
	if strings.EqualFold(toolName, "current_weather") {
		if _, ok := normalized["location"]; !ok {
			parts := []string{}
			for _, key := range []string{"city", "state", "country"} {
				if val, ok := normalized[key]; ok {
					if s := strings.TrimSpace(fmt.Sprint(val)); s != "" {
						parts = append(parts, s)
					}
				}
			}
			if len(parts) > 0 {
				normalized["location"] = strings.Join(parts, ", ")
			}
		}
	}
	return normalized
}

func parseToolArguments(raw json.RawMessage) (map[string]any, error) {
	args := map[string]any{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return args, nil
	}
	var lastErr error
	if err := json.Unmarshal(raw, &args); err == nil {
		return args, nil
	} else {
		lastErr = err
	}
	var argString string
	if err := json.Unmarshal(raw, &argString); err == nil {
		if strings.TrimSpace(argString) == "" {
			return args, nil
		}
		if err := json.Unmarshal([]byte(argString), &args); err == nil {
			return args, nil
		} else {
			return nil, fmt.Errorf("parse tool arguments string: %w", err)
		}
	} else {
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unexpected tool arguments format")
	}
	return nil, fmt.Errorf("parse tool arguments: %w", lastErr)
}

func (p *Provider) executeToolCalls(ctx context.Context, req providers.StreamRequest, calls []toolCall) (string, error) {
	if len(calls) == 0 {
		return "", nil
	}
	if req.ToolExecutor == nil {
		var summaries []string
		for _, call := range calls {
			summaries = append(summaries, fmt.Sprintf("[Tool call requested] %s args: %s", call.Function.Name, call.Function.Arguments))
		}
		return strings.Join(summaries, "\n"), nil
	}
	var outputs []string
	for _, call := range calls {
		args, err := parseToolArguments(call.Function.Arguments)
		if err != nil {
			return "", err
		}
		toolName := call.Function.Name
		if toolName == "" && len(req.Tools) == 1 {
			toolName = req.Tools[0].Name
		}
		if toolName == "" {
			for _, def := range req.Tools {
				if strings.EqualFold(def.Name, call.Function.Name) {
					toolName = def.Name
					break
				}
			}
		}
		args = normalizeToolArgs(toolName, args, req.Tools)
		if toolName == "" {
			if len(req.Tools) > 0 {
				toolName = req.Tools[0].Name
			} else {
				toolName = call.Function.Name
			}
		}
		if toolName == "" {
			toolName = call.Function.Name
		}
		result, err := req.ToolExecutor(ctx, toolName, args)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(result) != "" {
			outputs = append(outputs, fmt.Sprintf("[Tool %s]\n%s", toolName, result))
		}
	}
	return strings.Join(outputs, "\n\n"), nil
}

type streamChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role      string     `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []toolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done               bool  `json:"done"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

// LoadedModels returns the models currently loaded in memory on the host.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: /api/ps returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ps ollamaPsResponse
	if err := json.Unmarshal(body, &ps); err != nil {
		return nil, err
	}

	names := make([]string, len(ps.Models))
	for i, m := range ps.Models {
		names[i] = m.Name
	}
	return names, nil
}

// EnsureModelReady triggers a lightweight generate request to make sure the model is loaded.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	p.logTools(nil)
	payload := map[string]any{
		"model":  model,
		"prompt": ".",
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host.URL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama: /api/generate returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	return nil
}

// Stream issues a streaming chat request and forwards output to the provided callbacks.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	messages := req.History
	if req.SystemPrompt != "" {
		messages = append([]providers.ChatMessage{{Role: "system", Content: req.SystemPrompt}}, messages...)
	}

	streamEnabled := !req.DisableStreaming
	payload := map[string]any{
		"model":    req.Model,
		"messages": messages,
		"options":  req.Parameters,
		"stream":   streamEnabled,
	}

	p.logTools(req.Tools)

	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}

	if req.JSONMode {
		payload["format"] = "json"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	streamCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, req.Host.URL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if req.DisableStreaming && isNoToolCapabilityResponse(body) {
			if callbacks.OnChunk != nil {
				if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: "This model does not have tool capabilities."}); err != nil {
					return err
				}
			}
			if callbacks.OnComplete != nil {
				meta := providers.StreamMetadata{
					Model:     req.Model,
					CreatedAt: time.Now(),
					Done:      true,
				}
				if err := callbacks.OnComplete(meta); err != nil {
					return err
				}
			}
			return nil
		}
		return fmt.Errorf("ollama: /api/chat returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if !streamEnabled {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		var result streamChunk
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}
		output := result.Message.Content
		if len(result.Message.ToolCalls) > 0 {
			toolOutput, err := p.executeToolCalls(ctx, req, result.Message.ToolCalls)
			if err != nil {
				return err
			}
			if strings.TrimSpace(toolOutput) != "" {
				output = toolOutput
			}
		}
		if callbacks.OnChunk != nil && strings.TrimSpace(output) != "" {
			role := result.Message.Role
			if role == "" {
				role = "assistant"
			}
			if err := callbacks.OnChunk(providers.ChatMessage{Role: role, Content: output}); err != nil {
				return err
			}
		}
		if callbacks.OnComplete != nil {
			meta := providers.StreamMetadata{
				Model:              result.Model,
				CreatedAt:          time.Now(),
				Done:               true,
				TotalDuration:      result.TotalDuration,
				LoadDuration:       result.LoadDuration,
				PromptEvalCount:    result.PromptEvalCount,
				PromptEvalDuration: result.PromptEvalDuration,
				EvalCount:          result.EvalCount,
				EvalDuration:       result.EvalDuration,
			}
			if err := callbacks.OnComplete(meta); err != nil {
				return err
			}
		}
		return nil
	}

	decoder := json.NewDecoder(resp.Body)
	var final streamChunk
	for {
		var chunk streamChunk
		if err := decoder.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk(providers.ChatMessage{Role: chunk.Message.Role, Content: chunk.Message.Content}); err != nil {
				return err
			}
		}

		if chunk.Done {
			final = chunk
			break
		}
	}

	if callbacks.OnComplete != nil {
		meta := providers.StreamMetadata{
			Model:              final.Model,
			CreatedAt:          time.Now(),
			Done:               final.Done,
			TotalDuration:      final.TotalDuration,
			LoadDuration:       final.LoadDuration,
			PromptEvalCount:    final.PromptEvalCount,
			PromptEvalDuration: final.PromptEvalDuration,
			EvalCount:          final.EvalCount,
			EvalDuration:       final.EvalDuration,
		}
		if err := callbacks.OnComplete(meta); err != nil {
			return err
		}
	}

	return nil
}

// Close releases resources held by the provider.

func isNoToolCapabilityResponse(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(string(body)))
	if text != "" && strings.Contains(text, "tool") && (strings.Contains(text, "support") || strings.Contains(text, "capab")) {
		return true
	}
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		combined := strings.ToLower(strings.TrimSpace(payload.Error + " " + payload.Message))
		if combined != "" && strings.Contains(combined, "tool") && (strings.Contains(combined, "support") || strings.Contains(combined, "capab")) {
			return true
		}
	}
	return false
}

func (p *Provider) Close() error {
	return nil
}
