package benchmark

import (
	"context"
	"sync"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

type fakeProvider struct {
	mu          sync.Mutex
	ensureCalls int
	streamCalls int
	loadedCalls int
}

func (p *fakeProvider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loadedCalls++
	return []string{}, nil
}

func (p *fakeProvider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCalls++
	return nil
}

func (p *fakeProvider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	p.mu.Lock()
	p.streamCalls++
	p.mu.Unlock()

	if callbacks.OnChunk != nil {
		_ = callbacks.OnChunk(providers.ChatMessage{Role: "assistant", Content: "chunk"})
	}
	if callbacks.OnComplete != nil {
		_ = callbacks.OnComplete(providers.StreamMetadata{
			EvalCount:       4,
			PromptEvalCount: 3,
		})
	}
	return nil
}

func (p *fakeProvider) Close() error { return nil }

func TestBenchmarkModelsWithProviderFakes(t *testing.T) {
	origNewChatProvider := newChatProvider
	origUnloadModels := unloadModels
	origWriteResults := writeResultsFn
	t.Cleanup(func() {
		newChatProvider = origNewChatProvider
		unloadModels = origUnloadModels
		writeResultsFn = origWriteResults
	})

	fake := &fakeProvider{}
	newChatProvider = func(cfg *appconfig.Config) (providers.ChatProvider, error) {
		return fake, nil
	}

	unloadCalled := 0
	unloadModels = func(cfg *appconfig.Config) {
		unloadCalled++
	}

	var captured map[string]*BenchmarkResult
	writeResultsFn = func(results map[string]*BenchmarkResult, benchmarkCount int) error {
		captured = results
		return nil
	}

	cfg := &appconfig.Config{
		BenchmarkCount: 2,
		Hosts: []appconfig.Host{
			{Name: "h1", Models: []string{"m1"}},
			{Name: "h2", Models: []string{"m2"}},
		},
	}

	if err := BenchmarkModels(cfg); err != nil {
		t.Fatalf("BenchmarkModels error: %v", err)
	}
	if unloadCalled != 1 {
		t.Fatalf("expected unload models once, got %d", unloadCalled)
	}

	fake.mu.Lock()
	ensureCalls := fake.ensureCalls
	streamCalls := fake.streamCalls
	fake.mu.Unlock()

	if ensureCalls != 2 {
		t.Fatalf("expected EnsureModelReady called twice, got %d", ensureCalls)
	}
	if streamCalls != 4 {
		t.Fatalf("expected Stream called 4 times, got %d", streamCalls)
	}

	if len(captured) != 2 {
		t.Fatalf("expected results for two models, got %d", len(captured))
	}
	for name, result := range captured {
		if len(result.Iterations) != 2 {
			t.Fatalf("expected 2 iterations for %s, got %d", name, len(result.Iterations))
		}
		for _, iter := range result.Iterations {
			if iter.Stats.OutputTokenCount != 4 || iter.Stats.InputTokenCount != 3 {
				t.Fatalf("unexpected token counts: %+v", iter.Stats)
			}
		}
	}
}
