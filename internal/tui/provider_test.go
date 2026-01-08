// internal/tui/provider_test.go
package tui

import (
	"context"

	"github.com/mwiater/agon/internal/providers"
)

// testProvider is a mock implementation of the providers.ChatProvider interface for testing purposes.
type testProvider struct {
	loadedModels map[string][]string
	streamChunks []providers.ChatMessage
}

// newTestProvider creates a new instance of testProvider.
func newTestProvider() *testProvider {
	return &testProvider{loadedModels: make(map[string][]string)}
}

// LoadedModels returns a list of loaded models for a given host.
func (p *testProvider) LoadedModels(ctx context.Context, host Host) ([]string, error) {
	models := p.loadedModels[host.Name]
	out := make([]string, len(models))
	copy(out, models)
	return out, nil
}

// EnsureModelReady is a no-op for the test provider.
func (p *testProvider) EnsureModelReady(ctx context.Context, host Host, model string) error {
	return nil
}

// Stream simulates a chat stream, sending predefined chunks to the callbacks.
func (p *testProvider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	for _, msg := range p.streamChunks {
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk(msg); err != nil {
				return err
			}
		}
	}
	if callbacks.OnComplete != nil {
		return callbacks.OnComplete(providers.StreamMetadata{Done: true})
	}
	return nil
}

// Close is a no-op for the test provider.
func (p *testProvider) Close() error { return nil }
