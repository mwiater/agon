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

func (p *Provider) log(format string, args ...any) {
	logging.LogEvent(format, args...)
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
	logging.LogEvent("Tool requested: tool=%s host=%s model=%s args=%s", name, host, model, payload)
}

func (p *Provider) logToolSuccess(name, result, host, model string) {
	truncated := truncateForLog(result, 160)
	logging.LogEvent("Tool executed: tool=%s host=%s model=%s output=%s", name, host, model, truncated)
}

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
	hostName := hostLabel(req.Host)
	logging.LogEvent("[AGON->MCP] Incoming request metadata: user_prompt='%s', system_prompt='%s'", userPrompt, systemPrompt)
	toolName, userText := p.selectTool(req.History)
	executed := false
	forwardReq := req
	if len(p.toolDefs) > 0 {
		forwardReq.Tools = append([]providers.ToolDefinition(nil), p.toolDefs...)
	}

	// Replace system prompt for MCP mode
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
			logging.LogEvent("MCP tool attempt: tool=%s host=%s model=%s attempt=%d/%d", name, hostName, req.Model, attempt, retryLimit)
			p.logToolRequest(name, hostName, req.Model, wireArgs)
			result, err := p.callTool(toolCtx, hostName, req.Model, name, wireArgs)
			cancel()
			if err != nil {
				p.log("[ERROR] Tool bypassed: tool=%s host=%s model=%s reason=%v", name, hostName, req.Model, err)
				return "", err
			}
			if result.Retry && attempt < retryLimit {
				// Enter strict retry loop: only exit on success or reaching max attempts.
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(execCtx, req, name, result.Output, nextAttempt)
					if fixErr != nil {
						// If the context is done we should propagate the cancellation instead of retrying.
						if ctxErr := execCtx.Err(); ctxErr != nil {
							return "", ctxErr
						}
						if ctxErr := ctx.Err(); ctxErr != nil {
							return "", ctxErr
						}
						if errors.Is(fixErr, context.Canceled) || errors.Is(fixErr, context.DeadlineExceeded) {
							return "", fixErr
						}
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
						p.logToolSuccess(name, interp, hostName, req.Model)
						return interp, nil
					}
					p.logToolSuccess(name, fixedOut, hostName, req.Model)
					return fixedOut, nil
				}
				// Max attempts reached without success; fall through to return last known message.
			}
			retryState[name] = 0
			// Potentially interpret via LLM if server requested it.
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
				// Strict retry: loop until a successful tool call or max attempts.
				for attempt < retryLimit {
					nextAttempt := attempt + 1
					fixedOut, called, retryAgain, fixErr := p.fixWithLLMRoundTrip(ctx, req, toolName, result.Output, nextAttempt)
					if fixErr != nil {
						// Stop retrying when the context has already been cancelled.
						if ctxErr := ctx.Err(); ctxErr != nil {
							return ctxErr
						}
						if errors.Is(fixErr, context.Canceled) || errors.Is(fixErr, context.DeadlineExceeded) {
							return fixErr
						}
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

	//p.log("Last Message: %s", forwardReq.History[len(forwardReq.History)-1].Content)
	p.log("Forwarding request: host=%s model=%s messages=%d tools=%d", hostName, forwardReq.Model, len(forwardReq.History), len(forwardReq.Tools))
	err := p.fallback.Stream(ctx, forwardReq, callbacks)
	if err != nil {
		if !executed {
			p.log("[ERROR] Tool bypassed: tool=chat host=%s model=%s reason=%v", hostName, req.Model, err)
		}
		return err
	}
	if executed {
		p.log("Tool executed: tool=chat host=%s model=%s forwarded to Ollama", hostName, req.Model)
	} else {
		p.log("Tool bypassed: tool=chat host=%s model=%s reason=delegated to Ollama API", hostName, req.Model)
	}
	return nil
}
