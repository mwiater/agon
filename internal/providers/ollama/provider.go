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
	"regexp"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

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

func hostIdentifier(host appconfig.Host) string {
	name := strings.TrimSpace(host.Name)
	if name != "" {
		return name
	}
	if url := strings.TrimSpace(host.URL); url != "" {
		return url
	}
	return "ollama-host"
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
		argStringTrimmed := strings.TrimSpace(argString)
		if argStringTrimmed == "" {
			return args, nil
		}
		if err := json.Unmarshal([]byte(argStringTrimmed), &args); err == nil {
			return args, nil
		} else {
			lastErr = err
			sanitized := sanitizeLegacyJSON(argStringTrimmed)
			if sanitized != argStringTrimmed {
				if err := json.Unmarshal([]byte(sanitized), &args); err == nil {
					return args, nil
				}
			}
			return nil, fmt.Errorf("parse tool arguments string: %w", lastErr)
		}
	} else {
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unexpected tool arguments format")
	}
	return nil, fmt.Errorf("parse tool arguments: %w", lastErr)
}

var (
	singleQuotedStringPattern = regexp.MustCompile(`'([^']*)'`)
	trailingCommaPattern      = regexp.MustCompile(`,\s*([}\]])`)
)

func sanitizeLegacyJSON(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return s
	}
	replaced := singleQuotedStringPattern.ReplaceAllStringFunc(s, func(match string) string {
		if len(match) < 2 {
			return match
		}
		inner := match[1 : len(match)-1]
		inner = strings.ReplaceAll(inner, `"`, `\"`)
		return `"` + inner + `"`
	})
	cleaned := trailingCommaPattern.ReplaceAllString(replaced, "$1")
	return cleaned
}

func parseLegacyToolCalls(content string, available []providers.ToolDefinition) ([]toolCall, string) {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "<tool_call>")
	if idx == -1 {
		return nil, content
	}

	before := strings.TrimSpace(content[:idx])
	rest := content[idx+len("<tool_call>"):]

	endIdx := strings.Index(strings.ToLower(rest), "</tool_call>")
	var payload string
	var after string
	if endIdx == -1 {
		payload = rest
	} else {
		payload = rest[:endIdx]
		after = rest[endIdx+len("</tool_call>"):]
	}

	payload = strings.TrimSpace(payload)
	cleanedParts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(before); trimmed != "" {
		cleanedParts = append(cleanedParts, trimmed)
	}
	if trimmed := strings.TrimSpace(after); trimmed != "" {
		cleanedParts = append(cleanedParts, trimmed)
	}
	var cleaned string
	if len(cleanedParts) > 0 {
		cleaned = strings.Join(cleanedParts, "\n")
	}

	calls := buildLegacyToolCalls(payload, available, content)
	if len(calls) == 0 {
		return nil, content
	}
	return calls, cleaned
}

func buildLegacyToolCalls(payload string, available []providers.ToolDefinition, content string) []toolCall {
	if payload == "" {
		return nil
	}
	var raw any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		sanitized := sanitizeLegacyJSON(payload)
		if sanitized == payload {
			return nil
		}
		if err := json.Unmarshal([]byte(sanitized), &raw); err != nil {
			return nil
		}
	}

	var entries []any
	switch v := raw.(type) {
	case []any:
		entries = v
	case map[string]any:
		entries = []any{v}
	default:
		return nil
	}

	calls := make([]toolCall, 0, len(entries))
	for _, entry := range entries {
		call, ok := legacyEntryToToolCall(entry, available, content)
		if !ok {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func legacyEntryToToolCall(entry any, available []providers.ToolDefinition, content string) (toolCall, bool) {
	data, ok := entry.(map[string]any)
	if !ok {
		return toolCall{}, false
	}

	name := extractLegacyToolName(data)
	args := extractLegacyArguments(data)

	if fnMap, ok := data["function"].(map[string]any); ok {
		if innerName := extractLegacyToolName(fnMap); innerName != "" {
			name = innerName
		}
		if fnArgs := extractLegacyArguments(fnMap); fnArgs != nil {
			args = fnArgs
		}
	}

	if args == nil {
		args = map[string]any{}
	}

	resolvedName := resolveToolName(name, available, content)
	if resolvedName == "" && len(available) == 1 {
		resolvedName = available[0].Name
	}

	argBytes, err := json.Marshal(args)
	if err != nil {
		return toolCall{}, false
	}

	call := toolCall{Type: "function"}
	call.Function.Name = resolvedName
	call.Function.Arguments = json.RawMessage(argBytes)
	return call, true
}

func extractLegacyToolName(data map[string]any) string {
	candidates := []string{"name", "tool", "tool_name", "function"}
	for _, key := range candidates {
		if value, ok := data[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func extractLegacyArguments(data map[string]any) map[string]any {
	for _, key := range []string{"arguments", "params", "parameters"} {
		if raw, ok := data[key]; ok {
			if parsed, ok := coerceLegacyArguments(raw); ok {
				return parsed
			}
		}
	}
	return nil
}

func coerceLegacyArguments(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return map[string]any{}, true
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed, true
		}
		sanitized := sanitizeLegacyJSON(trimmed)
		if sanitized != trimmed {
			if err := json.Unmarshal([]byte(sanitized), &parsed); err == nil {
				return parsed, true
			}
		}
		return nil, false
	case json.RawMessage:
		return coerceLegacyArguments(string(v))
	case float64:
		return map[string]any{"value": v}, true
	case bool:
		return map[string]any{"value": v}, true
	default:
		if v == nil {
			return map[string]any{}, true
		}
	}
	return nil, false
}

func resolveToolName(candidate string, available []providers.ToolDefinition, content string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" {
		lowerCandidate := strings.ToLower(candidate)
		for _, tool := range available {
			if strings.ToLower(tool.Name) == lowerCandidate {
				return tool.Name
			}
		}
		for _, tool := range available {
			lowerTool := strings.ToLower(tool.Name)
			if strings.Contains(lowerTool, lowerCandidate) || strings.Contains(lowerCandidate, lowerTool) {
				return tool.Name
			}
		}
	}
	if len(available) == 1 {
		return available[0].Name
	}
	lowerContent := strings.ToLower(content)
	for _, tool := range available {
		if strings.Contains(lowerContent, strings.ToLower(tool.Name)) {
			return tool.Name
		}
	}
	return candidate
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
	logging.LogRequest("AGON->LLM", hostID, req.Model, "", body)

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
			toolOutput, err := p.executeToolCalls(ctx, req, toolCalls)
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

// findValidJSON searches a string for the first valid JSON object or array
// and returns it. If no valid JSON is found, it returns an empty string.
func findValidJSON(text string) string {
	// Iterate through the string to find a potential start of JSON
	for i := 0; i < len(text); i++ {
		char := text[i]

		// A valid JSON structure must start with '{' or '['
		if char == '{' || char == '[' {
			// Found a potential start, try to extract the full structure
			candidate := extractJSONStructure(text[i:])

			// If we got a non-empty candidate, check if it's valid JSON
			if candidate != "" && json.Valid([]byte(candidate)) {
				return candidate
			}
			// If it's not valid, the outer loop will continue searching
			// for the *next* '{' or '['
		}
	}
	// No valid JSON found in the entire string
	return ""
}

// extractJSONStructure attempts to find one complete, balanced JSON object or array
// starting from the beginning of the input string.
// It assumes the string starts with '{' or '['.
func extractJSONStructure(text string) string {
	if len(text) == 0 {
		return ""
	}

	var startChar, endChar byte
	if text[0] == '{' {
		startChar = '{'
		endChar = '}'
	} else if text[0] == '[' {
		startChar = '['
		endChar = ']'
	} else {
		// Not a valid start
		return ""
	}

	// level tracks the nesting of braces or brackets
	level := 0
	// inString tracks whether we are inside a string literal
	inString := false

	for i := 0; i < len(text); i++ {
		char := text[i]

		// Check for string literal boundaries
		if char == '"' {
			// We only toggle inString if the quote is not escaped
			if i == 0 || text[i-1] != '\\' {
				inString = !inString
			}
		}

		// Only count braces/brackets if we are not inside a string
		if !inString {
			if char == startChar {
				level++
			} else if char == endChar {
				level--
			}
		}

		// If level returns to 0, we've found the matching end
		if level == 0 {
			// Return the substring from the start to this point
			return text[0 : i+1]
		}
	}

	// If we finish the loop and level is not 0, the JSON is incomplete
	return ""
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

var errNoToolJSONFound = errors.New("no tool json found in response")

func rebuildToolCallFromContent(content string, tools []providers.ToolDefinition) (*toolCall, error) {
	if len(tools) == 0 {
		return nil, errNoToolJSONFound
	}
	candidates := extractToolCallCandidates(content)
	if len(candidates) == 0 {
		candidates = []string{content}
	}
	var firstErr error
	for _, candidate := range candidates {
		jsonCandidate := findValidJSON(candidate)
		if jsonCandidate == "" {
			trimmed := strings.TrimSpace(candidate)
			if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
				jsonCandidate = trimmed
			}
		}
		if jsonCandidate == "" {
			continue
		}
		parsed, err := parseJSONAnyWithSanitize(jsonCandidate)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("parse candidate json: %w", err)
			}
			continue
		}
		wrapper, candidateName, err := locateArgumentsWrapper(parsed)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		argsValue, ok := wrapper["arguments"]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("arguments key missing after normalization")
			}
			continue
		}
		argsMap, ok := argsValue.(map[string]any)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("arguments payload is not an object")
			}
			continue
		}
		matchedTool, err := matchToolDefinition(tools, candidateName, argsMap)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		argBytes, err := json.Marshal(argsMap)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("marshal arguments: %w", err)
			}
			continue
		}
		call := &toolCall{Type: "function"}
		call.Function.Name = matchedTool.Name
		call.Function.Arguments = json.RawMessage(argBytes)
		return call, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errNoToolJSONFound
}

func parseJSONAnyWithSanitize(input string) (any, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("empty json payload")
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		sanitized := sanitizeLegacyJSON(trimmed)
		if sanitized != trimmed {
			if err := json.Unmarshal([]byte(sanitized), &value); err == nil {
				return value, nil
			}
		}
		return nil, err
	}
	return value, nil
}

func locateArgumentsWrapper(root any) (map[string]any, string, error) {
	type queueItem struct {
		value    any
		toolName string
	}
	queue := []queueItem{{value: root}}
	visited := 0
	for len(queue) > 0 {
		if visited > 1024 {
			break
		}
		item := queue[0]
		queue = queue[1:]
		visited++
		switch v := item.value.(type) {
		case map[string]any:
			toolName := item.toolName
			if candidate := extractLegacyToolName(v); candidate != "" && strings.TrimSpace(toolName) == "" {
				toolName = candidate
			}
			if fn, ok := v["function"]; ok {
				queue = append(queue, queueItem{value: fn, toolName: toolName})
			}
			if call, ok := v["tool_call"]; ok {
				queue = append(queue, queueItem{value: call, toolName: toolName})
			}
			if calls, ok := v["tool_calls"]; ok {
				queue = append(queue, queueItem{value: calls, toolName: toolName})
			}
			if fnCall, ok := v["function_call"]; ok {
				queue = append(queue, queueItem{value: fnCall, toolName: toolName})
			}
			if argsRaw, ok := v["arguments"]; ok {
				if argsMap, ok := coerceLegacyArguments(argsRaw); ok {
					return map[string]any{"arguments": argsMap}, toolName, nil
				}
			}
			for _, inner := range v {
				switch inner.(type) {
				case map[string]any, []any, string, json.RawMessage:
					queue = append(queue, queueItem{value: inner, toolName: toolName})
				}
			}
		case []any:
			for _, inner := range v {
				queue = append(queue, queueItem{value: inner, toolName: item.toolName})
			}
		case json.RawMessage:
			if len(v) == 0 {
				return map[string]any{"arguments": map[string]any{}}, item.toolName, nil
			}
			queue = append(queue, queueItem{value: string(v), toolName: item.toolName})
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			parsed, err := parseJSONAnyWithSanitize(trimmed)
			if err == nil {
				queue = append(queue, queueItem{value: parsed, toolName: item.toolName})
			}
		}
	}
	return nil, "", errNoToolJSONFound
}

func matchToolDefinition(tools []providers.ToolDefinition, candidateName string, args map[string]any) (providers.ToolDefinition, error) {
	if len(tools) == 0 {
		return providers.ToolDefinition{}, errNoToolJSONFound
	}
	indices := prioritizeTools(tools, candidateName)
	var firstErr error
	for _, idx := range indices {
		tool := tools[idx]
		if err := validateArgumentsAgainstTool(tool, args); err == nil {
			return tool, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no matching tool for arguments")
	}
	return providers.ToolDefinition{}, firstErr
}

func prioritizeTools(tools []providers.ToolDefinition, candidateName string) []int {
	indices := make([]int, 0, len(tools))
	seen := make(map[int]struct{}, len(tools))
	if candidateName != "" {
		lowerCandidate := strings.ToLower(strings.TrimSpace(candidateName))
		for i, tool := range tools {
			if strings.ToLower(tool.Name) == lowerCandidate {
				indices = append(indices, i)
				seen[i] = struct{}{}
			}
		}
	}
	if len(indices) == 0 && len(tools) == 1 {
		return []int{0}
	}
	for i := range tools {
		if _, ok := seen[i]; ok {
			continue
		}
		indices = append(indices, i)
	}
	return indices
}

func validateArgumentsAgainstTool(def providers.ToolDefinition, args map[string]any) error {
	if len(def.InputSchema) == 0 {
		return nil
	}
	schemaLoader := gojsonschema.NewGoLoader(def.InputSchema)
	argBytes, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal arguments for validation: %w", err)
	}
	result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewBytesLoader(argBytes))
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}
	if result.Valid() {
		return nil
	}
	var details []string
	for _, desc := range result.Errors() {
		details = append(details, desc.String())
	}
	return fmt.Errorf("arguments failed validation: %s", strings.Join(details, "; "))
}

func extractToolCallCandidates(content string) []string {
	var candidates []string
	lower := strings.ToLower(content)
	startTag := "<tool_call>"
	endTag := "</tool_call>"
	offset := 0
	for {
		startIdx := strings.Index(lower[offset:], startTag)
		if startIdx == -1 {
			break
		}
		startIdx += offset
		payloadStart := startIdx + len(startTag)
		endIdx := strings.Index(lower[payloadStart:], endTag)
		if endIdx == -1 {
			segment := strings.TrimSpace(content[payloadStart:])
			if segment != "" {
				candidates = append(candidates, segment)
			}
			break
		}
		endIdx += payloadStart
		segment := strings.TrimSpace(content[payloadStart:endIdx])
		if segment != "" {
			candidates = append(candidates, segment)
		}
		offset = endIdx + len(endTag)
	}
	return candidates
}
