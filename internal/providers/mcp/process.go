package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/ollama"
)

// New spins up the MCP server process and performs the initialize handshake.
func New(ctx context.Context, cfg *appconfig.Config) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mcp provider requires non-nil config")
	}

    binary := cfg.MCPBinaryPath()

	if _, err := os.Stat(binary); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logging.LogEvent("MCP server start aborted: binary %q missing", binary)
			return nil, fmt.Errorf("mcp binary not found at %q", binary)
		}
		logging.LogEvent("MCP server start aborted: binary %q not accessible (%v)", binary, err)
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
		logging.LogEvent("MCP server failed to start: %v", err)
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	provider := &Provider{
		cfg:       cfg,
		cmd:       cmd,
		stdin:     stdin,
		reader:    bufio.NewReader(stdout),
		writer:    bufio.NewWriter(stdin),
		fallback:  ollama.New(cfg),
		rpcMeta:   make(map[string]rpcMetadata),
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
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    "agon-cli",
			"version": "dev",
		},
	}
	meta := rpcMetadata{host: p.defaultMCPHost(), method: "initialize"}
	if _, err := p.rpcCall(ctx, "initialize", params, meta); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
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
