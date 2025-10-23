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

func (p *Provider) logToolRequest(name string, args map[string]any) {
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

	cmd := exec.CommandContext(ctx, binary)
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

func (p *Provider) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := p.rpcCall(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}
	if len(resp.Result) == 0 {
		return "", nil
	}
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		return "", err
	}
	var parts []string
	for _, part := range payload.Content {
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
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
	p.log("Tool invoked: tool=chat host=%s model=%s messages=%d", req.Host.Name, req.Model, len(req.History))
	toolName, userText := p.selectTool(req.History)
	executed := false
	forwardReq := req
	if len(p.toolDefs) > 0 {
		forwardReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}
	forwardReq.DisableStreaming = true
	forwardReq.ToolExecutor = func(execCtx context.Context, name string, callArgs map[string]any) (string, error) {
		toolCtx, cancel := context.WithTimeout(execCtx, p.cfg.MCPInitTimeoutDuration())
		defer cancel()
		p.logToolRequest(name, callArgs)
		result, err := p.callTool(toolCtx, name, callArgs)
		if err != nil {
			p.log("Tool bypassed: tool=%s host=%s model=%s reason=%v", name, req.Host.Name, req.Model, err)
			return "", err
		}
		p.logToolSuccess(name, result, req.Host.Name, req.Model)
		return result, nil
	}
	if toolName != "" {
		toolCtx, cancel := context.WithTimeout(ctx, p.cfg.MCPInitTimeoutDuration())
		defer cancel()
		args := map[string]any{"text": userText}
		p.logToolRequest(toolName, args)
		result, err := p.callTool(toolCtx, toolName, args)
		if err != nil {
			p.log("Tool bypassed: tool=%s host=%s model=%s reason=%v", toolName, req.Host.Name, req.Model, err)
		} else {
			executed = true
			p.logToolSuccess(toolName, result, req.Host.Name, req.Model)
			output := fmt.Sprintf("[MCP %s]\n%s", toolName, result)
			if callbacks.OnChunk != nil {
				if err := callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: output}); err != nil {
					p.log("Tool output dispatch failed: %v", err)
				}
			}
			forwardHistory := append([]providers.ChatMessage{}, req.History...)
			forwardHistory = append(forwardHistory, providers.ChatMessage{Role: "assistant", Content: output})
			forwardReq.History = forwardHistory
		}
	}
	err := p.fallback.Stream(ctx, forwardReq, callbacks)
	if err != nil {
		if !executed {
			p.log("Tool bypassed: tool=chat host=%s model=%s reason=%v", req.Host.Name, req.Model, err)
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
