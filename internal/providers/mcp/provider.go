// internal/providers/mcp/provider.go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
)

// Provider implements the providers.ChatProvider interface by orchestrating
// a separate Multi-Chat-Provider (MCP) process and a fallback provider.
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
	rpcMetaMu sync.Mutex
	rpcMeta   map[string]rpcMetadata
	toolIndex map[string]providers.ToolDefinition
	toolDefs  []providers.ToolDefinition
}

// log logs an event using the global logger.
func (p *Provider) log(format string, args ...any) {
	logging.LogEvent(format, args...)
}

// truncateForLog truncates a string to a maximum number of runes for logging.
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

// formatArgs formats a map of arguments into a JSON string for logging.
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

// hostLabel returns a display label for a host, preferring the name over the URL.
func hostLabel(host appconfig.Host) string {
	name := strings.TrimSpace(host.Name)
	if name != "" {
		return name
	}
	if url := strings.TrimSpace(host.URL); url != "" {
		return url
	}
	return "local-mcp"
}

// lastUserPrompt extracts the content of the last user message from the chat history.
func lastUserPrompt(history []providers.ChatMessage) string {
	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToLower(history[i].Role) == "user" {
			return history[i].Content
		}
	}
	return ""
}

// logToolRequest logs a tool request event.
func (p *Provider) logToolRequest(name, host, model string, args map[string]any) {
	payload := formatArgs(args)
	logging.LogEvent("Tool requested: tool=%s host=%s model=%s args=%s", name, host, model, payload)
}

// logToolSuccess logs a successful tool execution event.
func (p *Provider) logToolSuccess(name, result, host, model string) {
	truncated := truncateForLog(result, 160)
	logging.LogEvent("Tool executed: tool=%s host=%s model=%s output=%s", name, host, model, truncated)
}

// defaultMCPHost returns the default MCP host identifier.
func (p *Provider) defaultMCPHost() string {
	if p.cfg != nil {
		if strings.TrimSpace(p.cfg.MCPBinary) != "" {
			return p.cfg.MCPBinary
		}
		if strings.TrimSpace(p.cfg.ConfigPath) != "" {
			return p.cfg.ConfigPath
		}
	}
	return "local-mcp"
}

// LoadedModels delegates to the underlying fallback provider to get the list of loaded models.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	p.log("Tool invoked: tool=loaded_models host=%s", host.Name)
	models, err := p.fallback.LoadedModels(ctx, host)
	if err != nil {
		p.log("Tool bypassed: tool=loaded_models host=%s reason=%v", host.Name, err)
		return nil, err
	}
	p.log("Tool bypassed: tool=loaded_models host=%s reason=delegated to llama.cpp API", host.Name)
	return models, nil
}

// EnsureModelReady delegates to the underlying fallback provider to ensure a model is ready.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	p.log("Tool invoked: tool=ensure_model host=%s model=%s", host.Name, model)
	if err := p.fallback.EnsureModelReady(ctx, host, model); err != nil {
		p.log("Tool bypassed: tool=ensure_model host=%s model=%s reason=%v", host.Name, model, err)
		return err
	}
	p.log("Tool bypassed: tool=ensure_model host=%s model=%s reason=delegated to llama.cpp API", host.Name, model)
	return nil
}

// Stream orchestrates the chat flow, deciding whether to invoke a tool via MCP or delegate to the fallback provider.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	userPrompt := lastUserPrompt(req.History)
	systemPrompt := req.SystemPrompt
	hostName := hostLabel(req.Host)
	logging.LogEvent("[AGON->MCP] Incoming request metadata: user_prompt='%s', system_prompt='%s'", userPrompt, systemPrompt)
	toolName, userText := p.selectTool(req.History)
	executed := false
	forwardReq := req
	if len(p.toolDefs) > 0 {
		forwardReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}

	newSystemPrompt := "You are a helpful assistant with access to the following tools. When the user asks a question, first determine if one of the tools can help."
	foundSystemPrompt := false
	for i, msg := range forwardReq.History {
		if msg.Role == "system" {
			forwardReq.History[i].Content = newSystemPrompt
			foundSystemPrompt = true
			break
		}
	}
	if !foundSystemPrompt {
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
			logging.LogEvent("MCP tool attempt: tool=%s host=%s model=%s attempt=%d/%d", name, hostName, req.Model, attempt, retryLimit)
			p.logToolRequest(name, hostName, req.Model, wireArgs)
			result, err := p.callTool(toolCtx, hostName, req.Model, name, wireArgs)
			cancel()
			if err != nil {
				p.log("[ERROR] Tool bypassed: tool=%s host=%s model=%s reason=%v", name, hostName, req.Model, err)
				return "", err
			}
			if result.Retry && attempt < retryLimit {
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(execCtx, req, name, result.Output, nextAttempt)
					if fixErr != nil {
						if ctxErr := execCtx.Err(); ctxErr != nil {
							return "", ctxErr
						}
						if ctxErr := ctx.Err(); ctxErr != nil {
							return "", ctxErr
						}
						if errors.Is(fixErr, context.Canceled) || errors.Is(fixErr, context.DeadlineExceeded) {
							return "", fixErr
						}
						continue
					}
					if !called {
						continue
					}
					attempt = nextAttempt
					retryState[name] = attempt
					if retryAgain && attempt < retryLimit {
						result.Output = fixedOut
						continue
					}
					retryState[name] = 0
					if interp, ok := p.maybeInterpretResult(execCtx, req, name, fixedOut); ok {
						p.logToolSuccess(name, interp, hostName, req.Model)
						return interp, nil
					}
					p.logToolSuccess(name, fixedOut, hostName, req.Model)
					return fixedOut, nil
				}
			}
			retryState[name] = 0
			if interp, ok := p.maybeInterpretResult(execCtx, req, name, result.Output); ok {
				p.logToolSuccess(name, interp, hostName, req.Model)
				return interp, nil
			}
			p.logToolSuccess(name, result.Output, hostName, req.Model)
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
			logging.LogEvent("MCP tool attempt: tool=%s host=%s model=%s attempt=%d/%d", toolName, hostName, req.Model, attempt, retryLimit)
			toolCtx, cancel := context.WithTimeout(ctx, p.cfg.MCPInitTimeoutDuration())
			p.logToolRequest(toolName, hostName, req.Model, args)
			result, err := p.callTool(toolCtx, hostName, req.Model, toolName, args)
			cancel()
			if err != nil {
				p.log("[ERROR] Tool bypassed: tool=%s host=%s model=%s reason=%v", toolName, hostName, req.Model, err)
				break
			}
			if result.Retry && attempt < retryLimit {
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(ctx, req, toolName, result.Output, nextAttempt)
					if fixErr != nil {
						if ctxErr := ctx.Err(); ctxErr != nil {
							return ctxErr
						}
						if errors.Is(fixErr, context.Canceled) || errors.Is(fixErr, context.DeadlineExceeded) {
							return fixErr
						}
						continue
					}
					if !called {
						continue
					}
					attempt = nextAttempt
					retryState[toolName] = attempt
					result.Output = fixedOut
					if retryAgain && attempt < retryLimit {
						continue
					}
					retryState[toolName] = 0
					break
				}
			}
			retryState[toolName] = 0
			executed = true
			if interp, ok := p.maybeInterpretResult(ctx, req, toolName, result.Output); ok {
				p.logToolSuccess(toolName, interp, hostName, req.Model)
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
				p.logToolSuccess(toolName, result.Output, hostName, req.Model)
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

	p.log("Forwarding request: host=%s model=%s messages=%d tools=%d", hostName, forwardReq.Model, len(forwardReq.History), len(forwardReq.Tools))
	err := p.fallback.Stream(ctx, forwardReq, callbacks)
	if err != nil {
		if !executed {
			p.log("[ERROR] Tool bypassed: tool=chat host=%s model=%s reason=%v", hostName, req.Model, err)
		}
		return err
	}
	if executed {
		p.log("Tool executed: tool=chat host=%s model=%s forwarded to llama.cpp", hostName, req.Model)
	} else {
		p.log("Tool bypassed: tool=chat host=%s model=%s reason=delegated to llama.cpp API", hostName, req.Model)
	}
	return nil
}
