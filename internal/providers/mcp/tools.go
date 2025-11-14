// internal/providers/mcp/tools.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
)

// toolLabel returns a label for a tool from RPC metadata, preferring the tool name over the method.
func toolLabel(meta rpcMetadata) string {
	if strings.TrimSpace(meta.tool) != "" {
		return meta.tool
	}
	if strings.TrimSpace(meta.method) != "" {
		return meta.method
	}
	return "unknown"
}

// discoverTools fetches the list of available tools from the MCP and populates the provider's tool index.
func (p *Provider) discoverTools() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.MCPInitTimeoutDuration())
	defer cancel()

	meta := rpcMetadata{host: p.defaultMCPHost(), method: "tools/list"}
	resp, err := p.rpcCall(ctx, "tools/list", nil, meta)
	if err != nil {
		return err
	}
	if len(resp.Result) == 0 {
		return nil
	}
	var payload struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description,omitempty"`
			Parameters  map[string]any `json:"parameters,omitempty"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		return err
	}
	toolDefs := make([]providers.ToolDefinition, 0, len(payload.Tools))
	p.toolIndex = make(map[string]providers.ToolDefinition, len(payload.Tools))
	var names []string
	for _, tool := range payload.Tools {
		def := providers.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
		key := strings.ToLower(tool.Name)
		p.toolIndex[key] = def
		toolDefs = append(toolDefs, def)
		names = append(names, tool.Name)
	}
	p.toolDefs = toolDefs
	if len(names) > 0 {
		p.log("Available MCP tools: %s", strings.Join(names, ", "))
	}
	return nil
}

// selectTool attempts to select a tool based on keywords in the user's chat history.
func (p *Provider) selectTool(history []providers.ChatMessage) (string, string) {
	if len(history) == 0 || len(p.toolIndex) == 0 {
		return "", ""
	}
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if strings.ToLower(msg.Role) != "user" {
			continue
		}
		content := msg.Content
		lower := strings.ToLower(content)
		for key, def := range p.toolIndex {
			if strings.Contains(lower, key) && strings.Contains(lower, "tool") {
				return def.Name, content
			}
		}
		break
	}
	return "", ""
}

// toolCallResponse represents the response from a tool call, including output and a retry flag.
type toolCallResponse struct {
	Output string
	Retry  bool
}

// callTool executes a tool via an RPC call to the MCP.
func (p *Provider) callTool(ctx context.Context, host, model, name string, args map[string]any) (toolCallResponse, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	meta := rpcMetadata{
		host:   host,
		model:  model,
		tool:   name,
		method: "tools/call",
	}
	resp, err := p.rpcCall(ctx, "tools/call", params, meta)
	if err != nil {
		return toolCallResponse{}, err
	}
	if len(resp.Result) == 0 {
		return toolCallResponse{}, nil
	}
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		return toolCallResponse{}, err
	}

	var (
		jsonPart      string
		interpretPart string
	)
	retryRequested := false
	var parts []string
	for _, part := range payload.Content {
		t := strings.ToLower(strings.TrimSpace(part.Type))
		switch t {
		case "json":
			jsonPart = part.Text
		case "interpret", "prompt":
			interpretPart = part.Text
		case "log":
			if strings.TrimSpace(part.Text) != "" {
				p.log("MCP tool detail: tool=%s %s", name, part.Text)
			}
			continue
		case "meta":
			if strings.TrimSpace(part.Text) == "retry" {
				retryRequested = true
			}
			continue
		}
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	if strings.TrimSpace(jsonPart) != "" && strings.TrimSpace(interpretPart) != "" {
		env := map[string]any{
			"__mcp_interpret__": true,
			"tool":              name,
			"json":              jsonPart,
			"prompt":            interpretPart,
		}
		data, err := json.Marshal(env)
		if err == nil {
			return toolCallResponse{Output: string(data)}, nil
		}
	}
	return toolCallResponse{Output: strings.Join(parts, "\n"), Retry: retryRequested}, nil
}

// fixWithLLMRoundTrip performs a one-off, non-streaming LLM request to correct and reissue a failing tool call.
func (p *Provider) fixWithLLMRoundTrip(ctx context.Context, req providers.StreamRequest, toolName, fixInstruction string, nextAttempt int) (output string, called bool, retryAgain bool, err error) {
	if err := ctx.Err(); err != nil {
		return "", false, false, err
	}
	hostName := hostLabel(req.Host)
	history := append([]providers.ChatMessage{}, req.History...)
	fixText := strings.TrimSpace(fixInstruction)
	if fixText == "" {
		fixText = "A previous tool call failed due to invalid or missing arguments. Please correct the arguments and call the tool again."
	}
	history = append(history, providers.ChatMessage{Role: "assistant", Content: fmt.Sprintf("[MCP %s error]\n%s", toolName, fixText)})
	history = append(history, providers.ChatMessage{Role: "user", Content: fmt.Sprintf("Call the %s tool again now with corrected arguments. Only call the tool; do not include extra text.", toolName)})

	fixReq := req
	fixReq.DisableStreaming = true

	if len(p.toolDefs) > 0 {
		fixReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}
	fixReq.History = history

	var tcResp toolCallResponse
	var tcErr error
	fixReq.ToolExecutor = func(execCtx context.Context, name string, args map[string]any) (string, error) {
		wireArgs := make(map[string]any, len(args)+3)
		for k, v := range args {
			wireArgs[k] = v
		}
		if _, ok := wireArgs["__user_prompt"]; !ok {
			if prompt := lastUserPrompt(req.History); prompt != "" {
				wireArgs["__user_prompt"] = prompt
			}
		}
		if nextAttempt > 0 {
			wireArgs["__mcp_attempt"] = nextAttempt
		}
		toolCtx, cancel := context.WithTimeout(execCtx, p.cfg.MCPInitTimeoutDuration())
		defer cancel()
		attemptLabel := nextAttempt
		if attemptLabel <= 0 {
			attemptLabel = 1
		}
		logging.LogEvent("MCP tool attempt: tool=%s host=%s model=%s attempt=%d/%d (fix round-trip)", name, hostName, req.Model, attemptLabel, p.cfg.MCPRetryAttempts())
		p.logToolRequest(name, hostName, req.Model, wireArgs)
		resp, err := p.callTool(toolCtx, hostName, req.Model, name, wireArgs)
		tcResp = resp
		tcErr = err
		if err != nil {
			p.log("[ERROR] Tool retry via LLM failed: tool=%s host=%s model=%s reason=%v", name, hostName, req.Model, err)
			return "", err
		}
		p.logToolSuccess(name, resp.Output, hostName, req.Model)
		return resp.Output, nil
	}

	var out strings.Builder
	cb := providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			out.WriteString(msg.Content)
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error { return nil },
	}
	start := time.Now()

	var toolNames []string
	for _, td := range fixReq.Tools {
		toolNames = append(toolNames, td.Name)
	}
	sendSummary := map[string]any{
		"fix_instruction":   strings.TrimSpace(fixText),
		"user_instruction":  fmt.Sprintf("Call the %s tool again now with corrected arguments. Only call the tool; do not include extra text.", toolName),
		"tools":             strings.Join(toolNames, ", "),
		"disable_streaming": true,
	}
	if data, err := json.Marshal(sendSummary); err == nil {
		logging.LogRequest("MCP->LLM", hostName, req.Model, toolName, data)
	} else {
		logging.LogEvent("MCP->LLM fix send: tool=%s host=%s model=%s", toolName, hostName, req.Model)
	}
	if err := p.fallback.Stream(ctx, fixReq, cb); err != nil {
		logging.LogEvent("MCP->LLM fix failed: tool=%s host=%s model=%s err=%v", toolName, hostName, req.Model, err)
		return "", false, false, err
	}
	dur := time.Since(start)
	fixed := strings.TrimSpace(out.String())

	recvPreview := truncateForLog(fixed, 500)
	logging.LogRequest("LLM->MCP", hostName, req.Model, toolName, map[string]any{
		"characters": len(fixed),
		"duration":   dur.String(),
		"preview":    recvPreview,
	})

	if tcErr == nil && (tcResp.Output != "" || tcResp.Retry) {
		return tcResp.Output, true, tcResp.Retry, nil
	}
	return fixed, false, false, nil
}

// maybeInterpretResult inspects a tool result for an MCP interpretation envelope and, if found, performs an LLM round-trip to generate a natural language summary.
func (p *Provider) maybeInterpretResult(ctx context.Context, req providers.StreamRequest, toolName, result string) (string, bool) {
	if !strings.Contains(result, "__mcp_interpret__") {
		return "", false
	}
	var env struct {
		Marker bool   `json:"__mcp_interpret__"`
		Tool   string `json:"tool"`
		JSON   string `json:"json"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(result), &env); err != nil || !env.Marker {
		return "", false
	}

	interpReq := req
	interpReq.DisableStreaming = true
	interpReq.Tools = nil
	history := append([]providers.ChatMessage{}, req.History...)
	jsonContent := strings.TrimSpace(env.JSON)
	if jsonContent == "" {
		jsonContent = "{}"
	}
	history = append(history, providers.ChatMessage{Role: "assistant", Content: fmt.Sprintf("[MCP %s JSON]\n%s", toolName, jsonContent)})
	prompt := strings.TrimSpace(env.Prompt)
	if prompt == "" {
		prompt = "Interpret the JSON above into a concise, natural language summary."
	}
	history = append(history, providers.ChatMessage{Role: "user", Content: prompt})
	interpReq.History = history

	var out strings.Builder
	start := time.Now()
	hostName := hostLabel(req.Host)
	logging.LogRequest("MCP->LLM", hostName, req.Model, toolName, map[string]any{
		"json":   jsonContent,
		"prompt": prompt,
	})
	cb := providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			out.WriteString(msg.Content)
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error { return nil },
	}
	if err := p.fallback.Stream(ctx, interpReq, cb); err != nil {
		logging.LogEvent("MCP->LLM interpret failed: tool=%s host=%s model=%s err=%v", toolName, hostName, req.Model, err)
		return "", false
	}
	dur := time.Since(start)
	interpreted := strings.TrimSpace(out.String())
	logging.LogRequest("LLM->MCP", hostName, req.Model, toolName, map[string]any{
		"characters": len(interpreted),
		"duration":   dur.String(),
		"output":     interpreted,
	})
	return interpreted, true
}
