// internal/providers/ollama/provider.go
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
	"github.com/mwiater/agon/internal/logging"
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

// ollamaPsResponse defines the structure of the response from the /api/ps endpoint.
type ollamaPsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// streamChunk defines the structure of a single chunk in a streaming response.
type streamChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role      string     `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []toolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	LogProbs           json.RawMessage `json:"logprobs,omitempty"`
	Done               bool  `json:"done"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

type generateResponse struct {
	Model              string          `json:"model"`
	Response           string          `json:"response"`
	Done               bool            `json:"done"`
	TotalDuration      int64           `json:"total_duration"`
	LoadDuration       int64           `json:"load_duration"`
	PromptEvalCount    int             `json:"prompt_eval_count"`
	PromptEvalDuration int64           `json:"prompt_eval_duration"`
	EvalCount          int             `json:"eval_count"`
	EvalDuration       int64           `json:"eval_duration"`
	LogProbs           json.RawMessage `json:"logprobs,omitempty"`
}

// LoadedModels returns the models currently loaded in memory on the host.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	endpoint := host.URL + "/api/ps"
	logging.LogRequest("AGON->LLM", hostIdentifier(host), "", "", map[string]string{"method": http.MethodGet, "url": endpoint})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
	logging.LogRequest("LLM->AGON", hostIdentifier(host), "", "", body)

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
	logTools(p.debug, nil)
	payload := map[string]any{
		"model": model,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	logging.LogRequest("AGON->LLM", hostIdentifier(host), model, "", body)

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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logging.LogRequest("LLM->AGON", hostIdentifier(host), model, "", respBody)

	if resp.StatusCode != http.StatusOK {
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
	hostID := hostIdentifier(req.Host)

	if len(messages) == 0 {
		messages = []providers.ChatMessage{}
	}

	if shouldUseGenerateForLogProbs(req) {
		return p.generateWithLogProbs(ctx, req, callbacks)
	}

	streamEnabled := !req.DisableStreaming
	options := buildOptions(req.Parameters)
	payload := map[string]any{
		"model":    req.Model,
		"messages": messages,
		"options":  options,
		"stream":   streamEnabled,
	}

	logTools(p.debug, req.Tools)

	if len(req.Tools) > 0 {
		payload["tools"] = formatToolsForPayload(req.Tools)
	}

	if req.JSONMode {
		payload["format"] = "json"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	if pretty, perr := json.MarshalIndent(payload, "", "  "); perr == nil {
		logging.LogRequest("AGON->LLM", hostID, req.Model, "", pretty)
	} else {
		logging.LogRequest("AGON->LLM", hostID, req.Model, "", body)
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
		logging.LogRequest("LLM->AGON", hostID, req.Model, "", body)
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
		logging.LogRequest("LLM->AGON", hostID, req.Model, "", body)
		var result streamChunk
		if err := json.Unmarshal(body, &result); err != nil {
			return err
		}
		output := result.Message.Content
		toolCalls := result.Message.ToolCalls
		if len(toolCalls) == 0 {
			if legacyCalls, cleaned := parseLegacyToolCalls(output, req.Tools); len(legacyCalls) > 0 {
				toolCalls = legacyCalls
				output = cleaned
			}
			if len(req.Tools) > 0 {
				call, err := rebuildToolCallFromContent(output, req.Tools)
				if err != nil {
					if p.debug && !errors.Is(err, errNoToolJSONFound) {
						log.Printf("ollama: unable to reconstruct tool call: %v", err)
					}
				} else if call != nil {
					toolCalls = []toolCall{*call}
					output = ""
				}
			}
		}
		if len(toolCalls) > 0 {
			toolOutput, err := executeToolCalls(ctx, req, toolCalls)
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
			modelName := result.Model
			if modelName == "" {
				modelName = req.Model
			}
			meta := providers.StreamMetadata{
				Model:              modelName,
				CreatedAt:          time.Now(),
				Done:               true,
				TotalDuration:      result.TotalDuration,
				LoadDuration:       result.LoadDuration,
				PromptEvalCount:    result.PromptEvalCount,
				PromptEvalDuration: result.PromptEvalDuration,
				EvalCount:          result.EvalCount,
				EvalDuration:       result.EvalDuration,
				LogProbs:           result.LogProbs,
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
		if data, err := json.Marshal(chunk); err == nil {
			logging.LogRequest("LLM->AGON", hostID, req.Model, "", data)
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
		modelName := final.Model
		if modelName == "" {
			modelName = req.Model
		}
		meta := providers.StreamMetadata{
			Model:              modelName,
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

func shouldUseGenerateForLogProbs(req providers.StreamRequest) bool {
	if !req.DisableStreaming {
		return false
	}
	if req.Parameters.LogProbs == nil || !*req.Parameters.LogProbs {
		return false
	}
	if len(req.Tools) > 0 || req.ToolExecutor != nil {
		return false
	}
	if len(req.History) != 1 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(req.History[0].Role), "user")
}

func (p *Provider) generateWithLogProbs(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	prompt := strings.TrimSpace(req.History[0].Content)
	payload := map[string]any{
		"model":   req.Model,
		"prompt":  prompt,
		"options": buildOptions(req.Parameters),
		"stream":  false,
	}
	if req.Parameters.LogProbs != nil {
		payload["logprobs"] = *req.Parameters.LogProbs
	}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		payload["system"] = req.SystemPrompt
	}
	if req.JSONMode {
		payload["format"] = "json"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	hostID := hostIdentifier(req.Host)
	logging.LogRequest("AGON->LLM", hostID, req.Model, "", body)

	streamCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, req.Host.URL+"/api/generate", bytes.NewReader(body))
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
		raw, _ := io.ReadAll(resp.Body)
		logging.LogRequest("LLM->AGON", hostID, req.Model, "", raw)
		return fmt.Errorf("ollama: /api/generate returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logging.LogRequest("LLM->AGON", hostID, req.Model, "", respBody)

	var result generateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	output := strings.TrimSpace(result.Response)
	if callbacks.OnChunk != nil && output != "" {
		if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: output}); err != nil {
			return err
		}
	}
	if callbacks.OnComplete != nil {
		modelName := result.Model
		if modelName == "" {
			modelName = req.Model
		}
		meta := providers.StreamMetadata{
			Model:              modelName,
			CreatedAt:          time.Now(),
			Done:               true,
			TotalDuration:      result.TotalDuration,
			LoadDuration:       result.LoadDuration,
			PromptEvalCount:    result.PromptEvalCount,
			PromptEvalDuration: result.PromptEvalDuration,
			EvalCount:          result.EvalCount,
			EvalDuration:       result.EvalDuration,
			LogProbs:           result.LogProbs,
		}
		if err := callbacks.OnComplete(meta); err != nil {
			return err
		}
	}

	return nil
}

func buildOptions(params appconfig.Parameters) map[string]any {
	options := map[string]any{}
	if params.TopK != nil {
		options["top_k"] = *params.TopK
	}
	if params.TopP != nil {
		options["top_p"] = *params.TopP
	}
	if params.MinP != nil {
		options["min_p"] = *params.MinP
	}
	if params.TFSZ != nil {
		options["tfs_z"] = *params.TFSZ
	}
	if params.TypicalP != nil {
		options["typical_p"] = *params.TypicalP
	}
	if params.RepeatLastN != nil {
		options["repeat_last_n"] = *params.RepeatLastN
	}
	if params.Temperature != nil {
		options["temperature"] = *params.Temperature
	}
	if params.RepeatPenalty != nil {
		options["repeat_penalty"] = *params.RepeatPenalty
	}
	if params.PresencePenalty != nil {
		options["presence_penalty"] = *params.PresencePenalty
	}
	if params.FrequencyPenalty != nil {
		options["frequency_penalty"] = *params.FrequencyPenalty
	}
	if params.LogProbs != nil {
		options["logprobs"] = *params.LogProbs
	}
	return options
}

// Close releases any resources held by the provider.
func (p *Provider) Close() error {
	return nil
}
