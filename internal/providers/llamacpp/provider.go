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
	ensureResponseFields(payload)
	ensureTimingsPerToken(payload)
	if req.JSONMode {
		payload["response_format"] = map[string]any{"type": "json_object"}
	}
	logTools(p.debug, req.Tools)
	if len(req.Tools) > 0 {
		payload["tools"] = formatToolsForPayload(req.Tools)
		payload["tool_choice"] = "auto"
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
		if req.DisableStreaming && isNoToolCapabilityResponse(raw) {
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
		return fmt.Errorf("llama.cpp: /v1/chat/completions returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	if req.DisableStreaming {
		return p.handleNonStreaming(ctx, resp, req, callbacks)
	}
	return p.handleStreaming(ctx, resp, req, callbacks)
}

func (p *Provider) handleNonStreaming(ctx context.Context, resp *http.Response, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logging.LogRequest("LLM->AGON", hostIdentifier(req.Host), req.Model, "", body)

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		raw := strings.TrimSpace(string(body))
		if strings.Contains(raw, "data:") {
			streamResp := &http.Response{Body: io.NopCloser(bytes.NewReader(body))}
			return p.handleStreaming(ctx, streamResp, req, callbacks)
		}
		if callbacks.OnChunk != nil && raw != "" {
			if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: raw}); err != nil {
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
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("llama.cpp: chat response contained no choices")
	}

	content := parsed.Choices[0].Message.Content
	role := parsed.Choices[0].Message.Role
	toolCalls := parsed.Choices[0].Message.ToolCalls
	if len(toolCalls) > 0 {
		toolOutput, err := executeToolCalls(ctx, req, toolCalls)
		if err != nil {
			return err
		}
		if strings.TrimSpace(toolOutput) != "" {
			content = toolOutput
		}
	}
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
		totalMs := parsed.Timings.PromptMs + parsed.Timings.PredictedMs
		meta := providers.StreamMetadata{
			Model:               modelName,
			CreatedAt:           time.Now(),
			Done:                true,
			TotalDuration:       msToNs(totalMs),
			LoadDuration:        0,
			PromptEvalCount:     parsed.Timings.PromptN,
			PromptTokens:        parsed.Usage.PromptTokens,
			PromptEvalDuration:  msToNs(parsed.Timings.PromptMs),
			EvalCount:           parsed.Timings.PredictedN,
			CompletionTokens:    parsed.Usage.CompletionTokens,
			EvalDuration:        msToNs(parsed.Timings.PredictedMs),
			TotalTokens:         parsed.Usage.TotalTokens,
			CacheN:              parsed.Timings.CacheN,
			PromptMs:            parsed.Timings.PromptMs,
			PromptPerTokenMs:    parsed.Timings.PromptPerTokenMs,
			PromptPerSecond:     parsed.Timings.PromptPerSecond,
			PredictedMs:         parsed.Timings.PredictedMs,
			PredictedPerTokenMs: parsed.Timings.PredictedPerTokenMs,
			PredictedPerSecond:  parsed.Timings.PredictedPerSecond,
			LogProbs:            parsed.Choices[0].LogProbs,
		}
		if err := callbacks.OnComplete(meta); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) handleStreaming(ctx context.Context, resp *http.Response, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	reader := bufio.NewReader(resp.Body)

	var finalModel string
	var toolCalls []toolCall
	loggedToolCalls := false
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
		if len(choice.Delta.ToolCalls) > 0 {
			toolCalls = append(toolCalls, choice.Delta.ToolCalls...)
		}
		if len(choice.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, choice.Message.ToolCalls...)
		}
		if len(toolCalls) > 0 && p.debug && !loggedToolCalls {
			logging.LogEvent("llama.cpp: streaming tool calls detected; executing after stream completion")
			loggedToolCalls = true
		}
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

	if len(toolCalls) > 0 {
		toolOutput, err := executeToolCalls(ctx, req, toolCalls)
		if err != nil {
			return err
		}
		if callbacks.OnChunk != nil && strings.TrimSpace(toolOutput) != "" {
			if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: toolOutput}); err != nil {
				return err
			}
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
	Timings struct {
		CacheN              int     `json:"cache_n"`
		PredictedMs         float64 `json:"predicted_ms"`
		PredictedN          int     `json:"predicted_n"`
		PredictedPerSecond  float64 `json:"predicted_per_second"`
		PredictedPerTokenMs float64 `json:"predicted_per_token_ms"`
		PromptMs            float64 `json:"prompt_ms"`
		PromptN             int     `json:"prompt_n"`
		PromptPerSecond     float64 `json:"prompt_per_second"`
		PromptPerTokenMs    float64 `json:"prompt_per_token_ms"`
	} `json:"timings"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		LogProbs json.RawMessage `json:"logprobs,omitempty"`
	} `json:"choices"`
}

type chatStreamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls,omitempty"`
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

func msToNs(ms float64) int64 {
	if ms <= 0 {
		return 0
	}
	return int64(ms * float64(time.Millisecond))
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

func applyParameters(payload map[string]any, params appconfig.LlamaParams) {
	if params.Temperature != nil {
		payload["temperature"] = *params.Temperature
	}
	if params.TopK != nil {
		payload["top_k"] = *params.TopK
	}
	if params.TopP != nil {
		payload["top_p"] = *params.TopP
	}
	if params.MinP != nil {
		payload["min_p"] = *params.MinP
	}
	if params.TypicalP != nil {
		payload["typical_p"] = *params.TypicalP
	}
	if params.DynaTempRange != nil {
		payload["dynatemp_range"] = *params.DynaTempRange
	}
	if params.DynaTempExponent != nil {
		payload["dynatemp_exponent"] = *params.DynaTempExponent
	}
	if params.Mirostat != nil {
		payload["mirostat"] = *params.Mirostat
	}
	if params.MirostatTau != nil {
		payload["mirostat_tau"] = *params.MirostatTau
	}
	if params.MirostatEta != nil {
		payload["mirostat_eta"] = *params.MirostatEta
	}
	if params.XTCProbability != nil {
		payload["xtc_probability"] = *params.XTCProbability
	}
	if params.XTCThreshold != nil {
		payload["xtc_threshold"] = *params.XTCThreshold
	}
	if params.Samplers != nil {
		payload["samplers"] = *params.Samplers
	}
	if params.RepeatLastN != nil {
		payload["repeat_last_n"] = *params.RepeatLastN
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
	if params.DryMultiplier != nil {
		payload["dry_multiplier"] = *params.DryMultiplier
	}
	if params.DryBase != nil {
		payload["dry_base"] = *params.DryBase
	}
	if params.DryAllowedLength != nil {
		payload["dry_allowed_length"] = *params.DryAllowedLength
	}
	if params.DryPenaltyLastN != nil {
		payload["dry_penalty_last_n"] = *params.DryPenaltyLastN
	}
	if params.DrySequenceBreakers != nil {
		payload["dry_sequence_breakers"] = *params.DrySequenceBreakers
	}
	if params.NPredict != nil {
		payload["n_predict"] = *params.NPredict
	}
	if params.Stop != nil {
		payload["stop"] = *params.Stop
	}
	if params.IgnoreEOS != nil {
		payload["ignore_eos"] = *params.IgnoreEOS
	}
	if params.TMaxPredictMS != nil {
		payload["t_max_predict_ms"] = *params.TMaxPredictMS
	}
	if params.Seed != nil {
		payload["seed"] = *params.Seed
	}
	if params.LogitBias != nil {
		payload["logit_bias"] = params.LogitBias
	}
	if params.NProbs != nil {
		payload["n_probs"] = *params.NProbs
	}
	if params.PostSamplingProbs != nil {
		payload["post_sampling_probs"] = *params.PostSamplingProbs
	}
	if params.ReturnTokens != nil {
		payload["return_tokens"] = *params.ReturnTokens
	}
	if params.MinKeep != nil {
		payload["min_keep"] = *params.MinKeep
	}
	if params.NKeep != nil {
		payload["n_keep"] = *params.NKeep
	}
	if params.CachePrompt != nil {
		payload["cache_prompt"] = *params.CachePrompt
	}
	if params.NCacheReuse != nil {
		payload["n_cache_reuse"] = *params.NCacheReuse
	}
	if params.Stream != nil {
		payload["stream"] = *params.Stream
	}
	if params.TimingsPerToken != nil {
		payload["timings_per_token"] = *params.TimingsPerToken
	}
	if params.ReturnProgress != nil {
		payload["return_progress"] = *params.ReturnProgress
	}
	if params.ResponseFields != nil {
		payload["response_fields"] = *params.ResponseFields
	}
	if params.NIndent != nil {
		payload["n_indent"] = *params.NIndent
	}
	if params.IDSlot != nil {
		payload["id_slot"] = *params.IDSlot
	}
	if params.Grammar != nil {
		payload["grammar"] = *params.Grammar
	}
	if params.JSONSchema != nil {
		payload["json_schema"] = params.JSONSchema
	}
	if params.NCmpl != nil {
		payload["n_cmpl"] = *params.NCmpl
	}
	if params.Lora != nil {
		payload["lora"] = *params.Lora
	}
}

func ensureResponseFields(payload map[string]any) {
	required := map[string]bool{
		"choices": true,
		"usage":   true,
		"timings": true,
	}
	if existing, ok := payload["response_fields"]; ok {
		switch fields := existing.(type) {
		case []string:
			seen := make(map[string]bool, len(fields))
			for _, f := range fields {
				seen[f] = true
			}
			for f := range required {
				if !seen[f] {
					fields = append(fields, f)
				}
			}
			payload["response_fields"] = fields
			return
		case []any:
			seen := make(map[string]bool, len(fields))
			out := make([]string, 0, len(fields)+len(required))
			for _, f := range fields {
				if s, ok := f.(string); ok {
					out = append(out, s)
					seen[s] = true
				}
			}
			for f := range required {
				if !seen[f] {
					out = append(out, f)
				}
			}
			payload["response_fields"] = out
			return
		}
	}
	payload["response_fields"] = []string{"choices", "usage", "timings"}
}

func ensureTimingsPerToken(payload map[string]any) {
	if _, ok := payload["timings_per_token"]; ok {
		return
	}
	payload["timings_per_token"] = true
}

type toolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func logTools(debug bool, tools []providers.ToolDefinition) {
	if !debug {
		return
	}
	if len(tools) == 0 {
		logging.LogEvent("llama.cpp tools: false")
		return
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Name != "" {
			names = append(names, tool.Name)
		}
	}
	if len(names) == 0 {
		logging.LogEvent("llama.cpp tools: false")
		return
	}
	logging.LogEvent("llama.cpp tools: {%s}", strings.Join(names, ", "))
}

func formatToolsForPayload(tools []providers.ToolDefinition) []map[string]any {
	formatted := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		function := map[string]any{
			"name": tool.Name,
		}
		if tool.Description != "" {
			function["description"] = tool.Description
		}
		if tool.Parameters != nil {
			function["parameters"] = tool.Parameters
		}
		formatted = append(formatted, map[string]any{
			"type":     "function",
			"function": function,
		})
	}
	return formatted
}

func executeToolCalls(ctx context.Context, req providers.StreamRequest, calls []toolCall) (string, error) {
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
		args = normalizeToolArgs(toolName, args, req.Tools)
		if toolName == "" {
			if len(req.Tools) > 0 {
				toolName = req.Tools[0].Name
			} else {
				toolName = call.Function.Name
			}
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
		argString = strings.TrimSpace(argString)
		if argString == "" {
			return args, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(argString), &parsed); err == nil {
			return parsed, nil
		}
		return nil, fmt.Errorf("parse tool arguments string: %w", err)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("invalid arguments payload")
	}
	return nil, fmt.Errorf("parse tool arguments: %w", lastErr)
}

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
