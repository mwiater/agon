// internal/providers/multiplex/provider.go
// Package multiplex routes provider calls based on host type.
package multiplex

import (
	"context"
	"fmt"
	"strings"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

// Provider delegates calls to an underlying provider based on host type.
type Provider struct {
	providers map[string]providers.ChatProvider
}

// New constructs a Provider from a map of host type to provider implementation.
func New(providerMap map[string]providers.ChatProvider) *Provider {
	normalized := make(map[string]providers.ChatProvider, len(providerMap))
	for key, provider := range providerMap {
		normalized[normalizeType(key)] = provider
	}
	return &Provider{providers: normalized}
}

// LoadedModels returns the models currently loaded in memory on the host.
func (p *Provider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	provider, err := p.providerForHost(host)
	if err != nil {
		return nil, err
	}
	return provider.LoadedModels(ctx, host)
}

// EnsureModelReady checks if a model is ready to be used and loads it if necessary.
func (p *Provider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	provider, err := p.providerForHost(host)
	if err != nil {
		return err
	}
	return provider.EnsureModelReady(ctx, host, model)
}

// Stream initiates a chat stream with the provider, sending and receiving messages.
func (p *Provider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	provider, err := p.providerForHost(req.Host)
	if err != nil {
		return err
	}
	return provider.Stream(ctx, req, callbacks)
}

// Close cleans up any resources used by the provider.
func (p *Provider) Close() error {
	var firstErr error
	seen := map[providers.ChatProvider]struct{}{}
	for _, provider := range p.providers {
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		if err := provider.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (p *Provider) providerForHost(host appconfig.Host) (providers.ChatProvider, error) {
	hostType := normalizeType(host.Type)
	if provider, ok := p.providers[hostType]; ok {
		return provider, nil
	}
	if hostType == "" {
		if provider, ok := p.providers["ollama"]; ok {
			return provider, nil
		}
	}
	return nil, fmt.Errorf("no provider registered for host type %q", host.Type)
}

func normalizeType(hostType string) string {
	normalized := strings.ToLower(strings.TrimSpace(hostType))
	switch normalized {
	case "", "ollama":
		return "ollama"
	case "llama.cpp", "llamacpp":
		return "llama.cpp"
	default:
		return normalized
	}
}
