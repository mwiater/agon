// Package mcp provides a ChatProvider that fronts a local MCP server process.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/mcplog"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/ollama"
)

type Provider struct {
	cfg       *appconfig.Config
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
	writer    *bufio.Writer
	seqMu     sync.Mutex
	seq       int64
	fallback  providers.ChatProvider
	rpcMu     sync.Mutex
	toolIndex map[string]providers.ToolDefinition
	toolDefs  []providers.ToolDefinition
}

func (p *Provider) log(format string, args ...any) {
	mcplog.Write(p.cfg, format, args...)
}

func truncateForLog(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	return string(runes[:max]) + "…"
}

func formatArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("%v", args)
	}
	return string(data)
}

func lastUserPrompt(history []providers.ChatMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToLower(history[i].Role) == "user" {
			return history[i].Content
		}
	}
	return ""
}

func (p *Provider) logToolRequest(name, host, model string, args map[string]any) {
	payload := formatArgs(args)
	p.log("Tool requested: tool=%s args=%s", name, payload)
	if p.cfg != nil && p.cfg.Debug {
		log.Printf("Tool request: %s %s", name, payload)
	}
}

func (p *Provider) logToolSuccess(name, result, host, model string) {
	truncated := truncateForLog(result, 160)
	p.log("Tool executed: tool=%s host=%s model=%s output=%s", name, host, model, truncated)
	if p.cfg != nil && p.cfg.Debug {
		log.Printf("Tool result: %s %s", name, result)
	}
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// New spins up the MCP server process and performs the initialize handshake.
func New(ctx context.Context, cfg *appconfig.Config) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mcp provider requires non-nil config")
	}

	binary := cfg.MCPBinary
	if binary == "" {
		binary = "dist/agon-mcp_linux_amd64_v1/agon-mcp"
	}

	if _, err := os.Stat(binary); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			mcplog.Write(cfg, "MCP server start aborted: binary %q missing", binary)
			return nil, fmt.Errorf("mcp binary not found at %q", binary)
		}
		mcplog.Write(cfg, "MCP server start aborted: binary %q not accessible (%v)", binary, err)
		return nil, fmt.Errorf("mcp binary %q not accessible: %w", binary, err)
	}

	cmd := exec.CommandContext(ctx, binary, "--config", cfg.ConfigPath)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		mcplog.Write(cfg, "MCP server failed to start: %v", err)
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	provider := &Provider{
		cfg:       cfg,
		cmd:       cmd,
		stdin:     stdin,
		reader:    bufio.NewReader(stdout),
		writer:    bufio.NewWriter(stdin),
		fallback:  ollama.New(cfg),
		toolIndex: make(map[string]providers.ToolDefinition),
	}

	initCtx, cancel := context.WithTimeout(ctx, cfg.MCPInitTimeoutDuration())
	defer cancel()

	if err := provider.initialize(initCtx); err != nil {
		provider.log("MCP server initialization failed: %v", err)
		provider.Close()
		return nil, err
	}

	if provider.cmd != nil && provider.cmd.Process != nil {
		provider.log("MCP server started: binary=%s pid=%d", binary, provider.cmd.Process.Pid)
	} else {
		provider.log("MCP server started: binary=%s", binary)
	}

	if err := provider.discoverTools(); err != nil {
		provider.log("Failed to list MCP tools: %v", err)
	}

	return provider, nil
}

func (p *Provider) initialize(ctx context.Context) error {
	id := p.nextID()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]any{
			"clientInfo": map[string]any{
				"name":    "agon-cli",
				"version": "dev",
			},
		},
	}

	if err := p.writeMessage(payload); err != nil {
		return fmt.Errorf("mcp initialize write: %w", err)
	}

	resp, err := p.readResponse(ctx)
	if err != nil {
		return fmt.Errorf("mcp initialize read: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("mcp initialize error: %s", resp.Error.Message)
	}
	return nil
}

func (p *Provider) nextID() int64 {
	p.seqMu.Lock()
	defer p.seqMu.Unlock()
	p.seq++
	return p.seq
}

func (p *Provider) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// This JSON-RPC frame is between agon and the MCP server over stdio.
	p.log("[agon -> mcp] Outgoing request: %s", string(data))
	if _, err := fmt.Fprintf(p.writer, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	if _, err := p.writer.Write(data); err != nil {
		return err
	}
	return p.writer.Flush()
}

func (p *Provider) readResponse(ctx context.Context) (jsonrpcResponse, error) {
	type result struct {
		resp jsonrpcResponse
		err  error
	}
	done := make(chan result, 1)
	go func() {
		r, err := p.readResponseBlocking()
		done <- result{resp: r, err: err}
	}()

	select {
	case <-ctx.Done():
		return jsonrpcResponse{}, ctx.Err()
	case res := <-done:
		return res.resp, res.err
	}
}

func (p *Provider) readResponseBlocking() (jsonrpcResponse, error) {
	headers := make(map[string]string)
	for {
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return jsonrpcResponse{}, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx >= 0 {
			headers[strings.ToLower(strings.TrimSpace(line[:idx]))] = strings.TrimSpace(line[idx+1:])
		}
	}

	cl, ok := headers["content-length"]
	if !ok {
		return jsonrpcResponse{}, fmt.Errorf("missing Content-Length header")
	}

	var length int
	if _, err := fmt.Sscanf(cl, "%d", &length); err != nil {
		return jsonrpcResponse{}, fmt.Errorf("invalid Content-Length: %w", err)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(p.reader, body); err != nil {
		return jsonrpcResponse{}, err
	}
	// This JSON-RPC frame is from the MCP server back to agon over stdio.
	p.log("[mcp -> agon] Incoming response: %s", string(body))

	var resp jsonrpcResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return jsonrpcResponse{}, err
	}
	return resp, nil
}

func (p *Provider) rpcCall(ctx context.Context, method string, params map[string]any) (jsonrpcResponse, error) {
	p.rpcMu.Lock()
	defer p.rpcMu.Unlock()

	id := p.nextID()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	if err := p.writeMessage(payload); err != nil {
		return jsonrpcResponse{}, err
	}
	resp, err := p.readResponse(ctx)
	if err != nil {
		return jsonrpcResponse{}, err
	}
	if resp.Error != nil {
		return jsonrpcResponse{}, fmt.Errorf("%s", resp.Error.Message)
	}
	return resp, nil
}

func (p *Provider) discoverTools() error {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.MCPInitTimeoutDuration())
	defer cancel()

	resp, err := p.rpcCall(ctx, "tools/list", nil)
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
			InputSchema map[string]any `json:"input_schema,omitempty"`
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
			InputSchema: tool.InputSchema,
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

type toolCallResponse struct {
	Output string
	Retry  bool
}

func (p *Provider) callTool(ctx context.Context, name string, args map[string]any) (toolCallResponse, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := p.rpcCall(ctx, "tools/call", params)
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
	// Detect a structured response that includes JSON + an interpretation prompt.
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
		// Return an envelope instructing the caller to perform an interpretation round-trip.
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
		// If marshaling fails, fall back to the plain join below.
	}
	return toolCallResponse{Output: strings.Join(parts, "\n"), Retry: retryRequested}, nil
}

// fixWithLLMRoundTrip performs a one-off, non-streaming LLM request that asks the
// model to correct and reissue the failing tool call. It threads the server's
// fix instructions along with the original user prompt, enables tools, and
// provides a ToolExecutor that executes the tool call without further retry
// loops. The returned string is the aggregated assistant output of that round.
// fixWithLLMRoundTrip requests the LLM to correct arguments and call the same
// tool again. It returns:
//  - output: the assistant output (either tool output when called, or raw text if no call happened)
//  - called: whether a tools/call was actually executed
//  - retryAgain: whether the tool's response indicated another retry is needed
//  - err: transport or provider error during the round-trip
// nextAttempt should be the attempt number to stamp into __mcp_attempt for the
// next tool invocation.
func (p *Provider) fixWithLLMRoundTrip(ctx context.Context, req providers.StreamRequest, toolName, fixInstruction string, nextAttempt int) (output string, called bool, retryAgain bool, err error) {
	// Compose a focused history to elicit a corrected tool call from the LLM.
	history := append([]providers.ChatMessage{}, req.History...)
	fixText := strings.TrimSpace(fixInstruction)
	if fixText == "" {
		fixText = "A previous tool call failed due to invalid or missing arguments. Please correct the arguments and call the tool again."
	}
	history = append(history, providers.ChatMessage{Role: "assistant", Content: fmt.Sprintf("[MCP %s error]\n%s", toolName, fixText)})
	history = append(history, providers.ChatMessage{Role: "user", Content: fmt.Sprintf("Call the %s tool again now with corrected arguments. Only call the tool; do not include extra text.", toolName)})

	fixReq := req
	fixReq.DisableStreaming = true
	// Re-enable tools so the LLM can produce a new tool call specification.
	if len(p.toolDefs) > 0 {
		fixReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}
	fixReq.History = history

	// Provide a ToolExecutor that executes exactly one call and reports whether
	// the tool requested another retry. Attempt counting is handled by caller.
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
		p.logToolRequest(name, req.Host.Name, req.Model, wireArgs)
		resp, err := p.callTool(toolCtx, name, wireArgs)
		tcResp = resp
		tcErr = err
		if err != nil {
			p.log("[ERROR] Tool retry via LLM failed: tool=%s host=%s model=%s reason=%v", name, req.Host.Name, req.Model, err)
			return "", err
		}
		// Defer interpretation to the outer retry controller; return raw output here.
		p.logToolSuccess(name, resp.Output, req.Host.Name, req.Model)
		return resp.Output, nil
	}

	// Capture the non-streaming assistant output of the corrective round-trip.
	var out strings.Builder
	cb := providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			out.WriteString(msg.Content)
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error { return nil },
	}
	start := time.Now()
	// Log what is being sent: the fix instruction, the user instruction, and enabled tools.
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
		p.log("MCP->LLM fix send: tool=%s host=%s model=%s payload=%s", toolName, req.Host.Name, req.Model, string(data))
	} else {
		p.log("MCP->LLM fix send: tool=%s host=%s model=%s", toolName, req.Host.Name, req.Model)
	}
	if err := p.fallback.Stream(ctx, fixReq, cb); err != nil {
		p.log("MCP->LLM fix failed: tool=%s host=%s model=%s err=%v", toolName, req.Host.Name, req.Model, err)
		return "", false, false, err
	}
	dur := time.Since(start)
	fixed := strings.TrimSpace(out.String())
	// Log what is received: the assistant text captured from the fix round-trip.
	recvPreview := truncateForLog(fixed, 500)
	p.log("MCP->LLM fix recv: tool=%s host=%s model=%s chars=%d dur=%s payload=%s", toolName, req.Host.Name, req.Model, len(fixed), dur.String(), recvPreview)
	// Report whether a tool call actually occurred and whether it asked for retry.
	if tcErr == nil && (tcResp.Output != "" || tcResp.Retry) {
		return tcResp.Output, true, tcResp.Retry, nil
	}
	return fixed, false, false, nil
}

// LoadedModels currently delegates to the underlying Ollama provider while the
// MCP toolchain is being fleshed out.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	p.log("Tool invoked: tool=loaded_models host=%s", host.Name)
	models, err := p.fallback.LoadedModels(ctx, host)
	if err != nil {
		p.log("Tool bypassed: tool=loaded_models host=%s reason=%v", host.Name, err)
		return nil, err
	}
	p.log("Tool bypassed: tool=loaded_models host=%s reason=delegated to Ollama API", host.Name)
	return models, nil
}

// EnsureModelReady currently proxies to the Ollama provider.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	p.log("Tool invoked: tool=ensure_model host=%s model=%s", host.Name, model)
	if err := p.fallback.EnsureModelReady(ctx, host, model); err != nil {
		p.log("Tool bypassed: tool=ensure_model host=%s model=%s reason=%v", host.Name, model, err)
		return err
	}
	p.log("Tool bypassed: tool=ensure_model host=%s model=%s reason=delegated to Ollama API", host.Name, model)
	return nil
}

// Stream proxies chat traffic through MCP tools before delegating to the Ollama backend.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	userPrompt := lastUserPrompt(req.History)
	systemPrompt := req.SystemPrompt
	p.log("[agon -> mcp] Incoming request: user_prompt='%s', system_prompt='%s'", userPrompt, systemPrompt)
	toolName, userText := p.selectTool(req.History)
	executed := false
	forwardReq := req
	if len(p.toolDefs) > 0 {
		forwardReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}

	// Replace system prompt for MCP mode
	newSystemPrompt := "You are a helpful assistant with access to the following tools. When the user asks a question, first determine if one of the tools can help. If so, call the tool with the required arguments. If not, answer the user's question directly. If the user's request is ambiguous, ask for clarification. For example, if they ask for the weather without a location, you must ask for a location."
	foundSystemPrompt := false
	for i, msg := range forwardReq.History {
		if msg.Role == "system" {
			forwardReq.History[i].Content = newSystemPrompt
			foundSystemPrompt = true
			break
		}
	}
	if !foundSystemPrompt {
		// Prepend if not found
		forwardReq.History = append([]providers.ChatMessage{{Role: "system", Content: newSystemPrompt}}, forwardReq.History...)
	}

	forwardReq.DisableStreaming = true
	retryState := make(map[string]int)
	retryLimit := p.cfg.MCPRetryAttempts()
	forwardReq.ToolExecutor = func(execCtx context.Context, name string, callArgs map[string]any) (string, error) {
		wireArgs := make(map[string]any, len(callArgs)+2)
		for k, v := range callArgs {
			wireArgs[k] = v
		}
		if _, ok := wireArgs["__user_prompt"]; !ok {
			if prompt := lastUserPrompt(req.History); prompt != "" {
				wireArgs["__user_prompt"] = prompt
			}
		}
		for {
			attempt := retryState[name]
			if attempt <= 0 {
				attempt = 1
			}
			retryState[name] = attempt
			wireArgs["__mcp_attempt"] = attempt
			toolCtx, cancel := context.WithTimeout(execCtx, p.cfg.MCPInitTimeoutDuration())
			p.logToolRequest(name, req.Host.Name, req.Model, wireArgs)
			result, err := p.callTool(toolCtx, name, wireArgs)
			cancel()
			if err != nil {
				p.log("[ERROR] Tool bypassed: tool=%s host=%s model=%s reason=%v", name, req.Host.Name, req.Model, err)
				return "", err
			}
			if result.Retry && attempt < retryLimit {
				// Enter strict retry loop: only exit on success or reaching max attempts.
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(execCtx, req, name, result.Output, nextAttempt)
					if fixErr != nil {
						// Fix round-trip failed (no call executed). Try again without consuming attempts.
						continue
					}
					if !called {
						// LLM did not issue a valid tool call; ask again.
						continue
					}
					// A tool call occurred; consume the attempt.
					attempt = nextAttempt
					retryState[name] = attempt
					if retryAgain && attempt < retryLimit {
						// Tool requested another retry; loop to elicit corrected args again.
						result.Output = fixedOut
						continue
					}
					// Either success or max reached; return output (interpreting if requested).
					retryState[name] = 0
					if interp, ok := p.maybeInterpretResult(execCtx, req, name, fixedOut); ok {
						p.logToolSuccess(name, interp, req.Host.Name, req.Model)
						return interp, nil
					}
					p.logToolSuccess(name, fixedOut, req.Host.Name, req.Model)
					return fixedOut, nil
				}
				// Max attempts reached without success; fall through to return last known message.
			}
			retryState[name] = 0
			// Potentially interpret via LLM if server requested it.
			if interp, ok := p.maybeInterpretResult(execCtx, req, name, result.Output); ok {
				p.logToolSuccess(name, interp, req.Host.Name, req.Model)
				return interp, nil
			}
			p.logToolSuccess(name, result.Output, req.Host.Name, req.Model)
			return result.Output, nil
		}
	}

	if toolName != "" {
		args := map[string]any{
			"text":          userText,
			"__user_prompt": userText,
		}
		for {
			attempt := retryState[toolName]
			if attempt <= 0 {
				attempt = 1
			}
			retryState[toolName] = attempt
			args["__mcp_attempt"] = attempt
			toolCtx, cancel := context.WithTimeout(ctx, p.cfg.MCPInitTimeoutDuration())
			p.logToolRequest(toolName, req.Host.Name, req.Model, args)
			result, err := p.callTool(toolCtx, toolName, args)
			cancel()
			if err != nil {
				p.log("[ERROR] Tool bypassed: tool=%s host=%s model=%s reason=%v", toolName, req.Host.Name, req.Model, err)
				break
			}
			if result.Retry && attempt < retryLimit {
				// Strict retry: loop until a successful tool call or max attempts.
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(ctx, req, toolName, result.Output, nextAttempt)
					if fixErr != nil {
						// Fix round-trip failed; try again without consuming attempts.
						continue
					}
					if !called {
						// No tool call executed; elicit again.
						continue
					}
					// Tool call executed; consume attempt and stash output for final handling.
					attempt = nextAttempt
					retryState[toolName] = attempt
					executed = true
					result.Output = fixedOut
					if retryAgain && attempt < retryLimit {
						// Tool asked to retry again; continue loop for another corrective round-trip.
						continue
					}
					// Either success or max reached; stop retrying.
					retryState[toolName] = 0
					break
				}
				// Continue with forwarding after loop.
			}
			retryState[toolName] = 0
			executed = true
			// If the result is an interpret envelope, perform the interpretation round-trip first.
			if interp, ok := p.maybeInterpretResult(ctx, req, toolName, result.Output); ok {
				p.logToolSuccess(toolName, interp, req.Host.Name, req.Model)
				output := fmt.Sprintf("[MCP %s] %s", toolName, strings.TrimSpace(interp))
				if callbacks.OnChunk != nil {
					if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: output}); err != nil {
						p.log("[ERROR] Tool output dispatch failed: %v", err)
					}
				}
				forwardHistory := append([]providers.ChatMessage{}, req.History...)
				forwardHistory = append(forwardHistory, providers.ChatMessage{Role: "assistant", Content: output})
				forwardReq.History = forwardHistory
			} else {
				p.logToolSuccess(toolName, result.Output, req.Host.Name, req.Model)
				output := fmt.Sprintf("[MCP %s] %s", toolName, result.Output)
				if callbacks.OnChunk != nil {
					if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: output}); err != nil {
						p.log("[ERROR] Tool output dispatch failed: %v", err)
					}
				}
				forwardHistory := append([]providers.ChatMessage{}, req.History...)
				forwardHistory = append(forwardHistory, providers.ChatMessage{Role: "assistant", Content: output})
				forwardReq.History = forwardHistory
			}
			break
		}
	}

	//p.log("Last Message: %s", forwardReq.History[len(forwardReq.History)-1].Content)
	p.log("Forwarding request: host=%s model=%s messages=%d tools=%d", forwardReq.Host.Name, forwardReq.Model, len(forwardReq.History), len(forwardReq.Tools))
	err := p.fallback.Stream(ctx, forwardReq, callbacks)
	if err != nil {
		if !executed {
			p.log("[ERROR] Tool bypassed: tool=chat host=%s model=%s reason=%v", req.Host.Name, req.Model, err)
		}
		return err
	}
	if executed {
		p.log("Tool executed: tool=chat host=%s model=%s forwarded to Ollama", req.Host.Name, req.Model)
	} else {
		p.log("Tool bypassed: tool=chat host=%s model=%s reason=delegated to Ollama API", req.Host.Name, req.Model)
	}
	return nil
}

// maybeInterpretResult inspects a tool result string for an MCP interpretation
// envelope. If found, it performs a non-streaming LLM request to turn the JSON
// into natural language and returns that text. The boolean indicates whether an
// interpretation was performed.
func (p *Provider) maybeInterpretResult(ctx context.Context, req providers.StreamRequest, toolName, result string) (string, bool) {
	// Quick check for marker to avoid unnecessary JSON parse.
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
	// Build a one-off, non-streaming chat to obtain a natural-language interpretation.
	interpReq := req
	interpReq.DisableStreaming = true
	interpReq.Tools = nil // disable tools for the interpretation round
	// Compose a short history: prior convo + assistant with JSON + user with prompt.
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

	// Set up local capture for the response.
	var out strings.Builder
	start := time.Now()
	p.log("MCP->LLM interpret send: tool=%s host=%s model=%s bytes_json=%d", toolName, req.Host.Name, req.Model, len(jsonContent))
	cb := providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			out.WriteString(msg.Content)
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error { return nil },
	}
	if err := p.fallback.Stream(ctx, interpReq, cb); err != nil {
		p.log("MCP->LLM interpret failed: tool=%s host=%s model=%s err=%v", toolName, req.Host.Name, req.Model, err)
		return "", false
	}
	dur := time.Since(start)
	interpreted := strings.TrimSpace(out.String())
	p.log("MCP->LLM interpret recv: tool=%s host=%s model=%s chars=%d dur=%s", toolName, req.Host.Name, req.Model, len(interpreted), dur.String())
	return interpreted, true
}

// Close terminates the MCP process and closes any subordinate providers.
func (p *Provider) Close() error {
	var firstErr error

	if p.stdin != nil {
		_ = p.stdin.Close()
	}

	if p.cmd != nil {
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()
		select {
		case err := <-done:
			if err != nil && firstErr == nil {
				firstErr = err
			}
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			if err := <-done; err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	if p.fallback != nil {
		if err := p.fallback.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
