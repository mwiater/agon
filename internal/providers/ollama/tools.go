// internal/providers/ollama/tools.go
package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/xeipuuv/gojsonschema"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

// logTools logs the available tool names if debug mode is enabled.
func logTools(debug bool, tools []providers.ToolDefinition) {
	if !debug {
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

// formatToolsForPayload converts a slice of ToolDefinition into a format suitable for the Ollama API payload.
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

// hostIdentifier returns a string identifier for a given host, preferring the name over the URL.
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

// toolCall represents a structured tool call from the Ollama API.
type toolCall struct {
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// normalizeToolArgs standardizes tool arguments, for example, by creating a 'location' from city, state, and country for the 'current_weather' tool.
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

// parseToolArguments attempts to parse a raw JSON message into a map of tool arguments.
func parseToolArguments(raw json.RawMessage) (map[string]any, error) {
	args := map[string]any{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return args, nil
	}
	if err := json.Unmarshal(raw, &args); err == nil {
		return args, nil
	}
	var argString string
	if err := json.Unmarshal(raw, &argString); err == nil {
		argStringtrimmed := strings.TrimSpace(argString)
		if argStringtrimmed == "" {
			return args, nil
		}
		var lastErr error
		if err := json.Unmarshal([]byte(argStringtrimmed), &args); err == nil {
			return args, nil
		} else {
			lastErr = err
			sanitized := sanitizeLegacyJSON(argStringtrimmed)
			if sanitized != argStringtrimmed {
				if err := json.Unmarshal([]byte(sanitized), &args); err == nil {
					return args, nil
				}
			}
			return nil, fmt.Errorf("parse tool arguments string: %w", lastErr)
		}
	} else {
		return nil, fmt.Errorf("parse tool arguments: %w", err)
	}
}

var (
	singleQuotedStringPattern = regexp.MustCompile(`'([^']*)'`)
	trailingCommaPattern      = regexp.MustCompile(`,\s*([}\]])`)
)

var legacyArgsBareValuePattern = regexp.MustCompile(`"arguments"\s*:\s*{\s*"[^":}]+"\s*}\n?`)

// sanitizeLegacyJSON cleans up common JSON-like syntax errors, such as single quotes and trailing commas.
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
		inner = strings.ReplaceAll(inner, "\"", `"`)
		return `"` + inner + `"`
	})
	cleaned := trailingCommaPattern.ReplaceAllString(replaced, "$1")
	return cleaned
}

// parseLegacyToolCalls extracts tool calls from a string that uses legacy <tool_call> tags.
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

// buildLegacyToolCalls constructs a slice of toolCall from a raw payload string.
func buildLegacyToolCalls(payload string, available []providers.ToolDefinition, content string) []toolCall {
	if payload == "" {
		return nil
	}
	payload = normalizeLegacyToolCallPayload(payload)
	var raw any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		sanitized := sanitizeLegacyJSON(payload)
		if sanitized == payload {
			return parseLooseLegacyToolCalls(payload, available, content)
		}
		if err := json.Unmarshal([]byte(sanitized), &raw); err != nil {
			return parseLooseLegacyToolCalls(sanitized, available, content)
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

// legacyEntryToToolCall converts a single legacy tool call entry into a structured toolCall.
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

// extractLegacyToolName finds a tool name from a map of data, checking common keys.
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

// extractLegacyArguments finds tool arguments from a map of data, checking common keys.
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

// coerceLegacyArguments attempts to convert a value of unknown type into a map of arguments.
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

// resolveToolName attempts to find the correct tool name from the available tools based on a candidate name.
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

// executeToolCalls executes a slice of tool calls and returns the combined output.
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

// findValidJSON searches a string for the first valid JSON object or array and returns it.
func findValidJSON(text string) string {
	for i := 0; i < len(text); i++ {
		char := text[i]
		if char == '{' || char == '[' {
			candidate := extractJSONStructure(text[i:])
			if candidate != "" && json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}
	return ""
}

// extractJSONStructure extracts a complete JSON object or array from the beginning of a string.
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
		return ""
	}

	level := 0
	inString := false

	for i := 0; i < len(text); i++ {
		char := text[i]
		if char == '"' {
			if i == 0 || text[i-1] != '\\' {
				inString = !inString
			}
		}
		if !inString {
			if char == startChar {
				level++
			} else if char == endChar {
				level--
			}
		}
		if level == 0 {
			return text[0 : i+1]
		}
	}
	return ""
}

// isNoToolCapabilityResponse checks if the response body indicates that the model does not support tools.
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

var errNoToolJSONFound = errors.New("no tool json found in response")

// rebuildToolCallFromContent attempts to reconstruct a tool call from a string of content.
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

// parseJSONAnyWithSanitize parses a JSON string into an 'any' type, attempting to sanitize it first if the initial parse fails.
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

// locateArgumentsWrapper traverses a nested data structure to find the map containing tool arguments.
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

// matchToolDefinition finds the best matching tool definition for a given candidate name and arguments.
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

// prioritizeTools returns a slice of tool indices, prioritized by how well they match the candidate name.
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

// validateArgumentsAgainstTool checks if the given arguments are valid for the specified tool definition.
func validateArgumentsAgainstTool(def providers.ToolDefinition, args map[string]any) error {
	if len(def.Parameters) == 0 {
		return nil
	}
	schemaLoader := gojsonschema.NewGoLoader(def.Parameters)
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

// extractToolCallCandidates finds all potential tool call payloads within <tool_call> tags.
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

// normalizeLegacyToolCallPayload makes a simple correction to a legacy tool call payload.
func normalizeLegacyToolCallPayload(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}
	return legacyArgsBareValuePattern.ReplaceAllString(input, "\"arguments\":{}\n")
}

// parseLooseLegacyToolCalls parses a string containing loosely-formatted legacy tool calls.
func parseLooseLegacyToolCalls(payload string, available []providers.ToolDefinition, content string) []toolCall {
	entries := splitLooseLegacyEntries(payload)
	if len(entries) == 0 {
		return nil
	}
	calls := make([]toolCall, 0, len(entries))
	for _, entry := range entries {
		if call, ok := parseLooseLegacyEntry(entry, available, content); ok {
			calls = append(calls, call)
		}
	}
	return calls
}

// splitLooseLegacyEntries splits a string into multiple, loosely-formatted JSON-like entries.
func splitLooseLegacyEntries(payload string) []string {
	s := strings.TrimSpace(payload)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && len(s) >= 2 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	var entries []string
	for i := 0; i < len(s); {
		if s[i] == '{' {
			segment := extractJSONStructure(s[i:])
			if segment == "" {
				break
			}
			entries = append(entries, segment)
			i += len(segment)
			for i < len(s) && (s[i] == ',' || s[i] == '}' || s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
				i++
			}
			continue
		}
		i++
	}
	if len(entries) == 0 && strings.Contains(s, "name") {
		entries = append(entries, s)
	}
	return entries
}

// parseLooseLegacyEntry parses a single, loosely-formatted legacy tool call entry.
func parseLooseLegacyEntry(entry string, available []providers.ToolDefinition, content string) (toolCall, bool) {
	text := strings.TrimSpace(entry)
	if text == "" {
		return toolCall{}, false
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err == nil {
		return legacyEntryToToolCall(data, available, content)
	}

	name := extractFirstMatch(entry, []string{"`name`", "`tool`", "`tool_name`", "`function`"})
	if name == "" {
		return toolCall{}, false
	}
	argsMap := parseLooseArguments(entry)
	call := toolCall{Type: "function"}
	call.Function.Name = resolveToolName(name, available, content)
	if call.Function.Name == "" && len(available) == 1 {
		call.Function.Name = available[0].Name
	}
	if call.Function.Name == "" {
		call.Function.Name = name
	}
	if argsMap == nil {
		argsMap = map[string]any{}
	}
	argBytes, err := json.Marshal(argsMap)
	if err != nil {
		return toolCall{}, false
	}
	call.Function.Arguments = json.RawMessage(argBytes)
	return call, true
}

// extractFirstMatch finds the first value associated with a list of keys in a string.
func extractFirstMatch(entry string, keys []string) string {
	for _, key := range keys {
		safeKey := regexp.QuoteMeta(key)
		pattern := fmt.Sprintf("`%s`%s", safeKey, `\s*:\s*"([^"]+)"`)

		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(entry); len(matches) == 2 {
			return matches[1]
		}
	}
	return ""
}

// parseLooseArguments extracts arguments from a loosely-formatted string.
func parseLooseArguments(entry string) map[string]any {
	idx := strings.Index(strings.ToLower(entry), "`arguments`")
	if idx == -1 {
		return map[string]any{}
	}
	sub := entry[idx:]
	startBrace := strings.Index(sub, "{")
	if startBrace == -1 {
		return map[string]any{}
	}
	structure := extractJSONStructure(sub[startBrace:])
	if strings.TrimSpace(structure) == "" {
		return map[string]any{}
	}
	parsed, err := parseJSONAnyWithSanitize(structure)
	if err != nil {
		return map[string]any{}
	}
	args, ok := coerceLegacyArguments(parsed)
	if !ok || args == nil {
		return map[string]any{}
	}
	return args
}
