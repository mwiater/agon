// Package ollama provides a ChatProvider backed by Ollama-compatible HTTP endpoints.
package ollama

import (
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
	"github.com/mwiater/agon/internal/providers"
)

// Provider implements the providers.ChatProvider interface using Ollama HTTP APIs.
type Provider struct {
	client  *http.Client
	timeout time.Duration
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
	}
}

type ollamaPsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type streamChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL+"/api/ps", nil)
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
	payload := map[string]any{
		"model":  model,
		"prompt": ".",
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
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

	payload := map[string]any{
		"model":    req.Model,
		"messages": messages,
		"options":  req.Parameters,
		"stream":   true,
	}

	if req.JSONMode {
		payload["format"] = "json"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama: /api/chat returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
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

// Close releases resources held by the provider.
func (p *Provider) Close() error {
	return nil
}
