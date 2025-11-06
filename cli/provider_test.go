package cli

import (
	"context"

	"github.com/mwiater/agon/internal/providers"
)

type testProvider struct {
	loadedModels map[string][]string
	streamChunks []providers.ChatMessage
}

func newTestProvider() *testProvider {
	return &testProvider{loadedModels: make(map[string][]string)}
}

func (p *testProvider) LoadedModels(ctx context.Context, host Host) ([]string, error) {
	models := p.loadedModels[host.Name]
	out := make([]string, len(models))
	copy(out, models)
	return out, nil
}

func (p *testProvider) EnsureModelReady(ctx context.Context, host Host, model string) error {
	return nil
}

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

func (p *testProvider) Close() error { return nil }
