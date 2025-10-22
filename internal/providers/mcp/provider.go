// Package mcp provides a ChatProvider that fronts a local MCP server process.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/ollama"
)

type Provider struct {
	cfg      *appconfig.Config
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	reader   *bufio.Reader
	writer   *bufio.Writer
	seqMu    sync.Mutex
	seq      int64
	fallback providers.ChatProvider
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
		binary = "dist/agon-mcp"
	}

	if _, err := os.Stat(binary); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("mcp binary not found at %q", binary)
		}
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
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	provider := &Provider{
		cfg:      cfg,
		cmd:      cmd,
		stdin:    stdin,
		reader:   bufio.NewReader(stdout),
		writer:   bufio.NewWriter(stdin),
		fallback: ollama.New(cfg),
	}

	initCtx, cancel := context.WithTimeout(ctx, cfg.MCPInitTimeoutDuration())
	defer cancel()

	if err := provider.initialize(initCtx); err != nil {
		provider.Close()
		return nil, err
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

// LoadedModels currently delegates to the underlying Ollama provider while the
// MCP toolchain is being fleshed out.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	return p.fallback.LoadedModels(ctx, host)
}

// EnsureModelReady currently proxies to the Ollama provider.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	return p.fallback.EnsureModelReady(ctx, host, model)
}

// Stream currently proxies to the Ollama provider. The MCP transport will be
// wired up in a follow-up step when the server exposes responses/create.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	return p.fallback.Stream(ctx, req, callbacks)
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
