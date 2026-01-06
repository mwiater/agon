// internal/providers/multiplex/provider_test.go
package multiplex

import (
	"context"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

type stubProvider struct {
	loadedCalled bool
	streamCalled bool
	closeCalled  int
}

func (s *stubProvider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	s.loadedCalled = true
	return []string{"model-a"}, nil
}

func (s *stubProvider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	return nil
}

func (s *stubProvider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	s.streamCalled = true
	return nil
}

func (s *stubProvider) Close() error {
	s.closeCalled++
	return nil
}

func TestNormalizeType(t *testing.T) {
	tests := map[string]string{
		"":          "llama.cpp",
		"llama.cpp": "llama.cpp",
		"llamacpp":  "llama.cpp",
		" LLAMA.CPP ": "llama.cpp",
		"custom":    "custom",
	}

	for input, want := range tests {
		if got := normalizeType(input); got != want {
			t.Fatalf("normalizeType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProviderForHostDefaults(t *testing.T) {
	stub := &stubProvider{}
	p := New(map[string]providers.ChatProvider{
		"llama.cpp": stub,
	})

	host := appconfig.Host{Type: "", Name: "default"}
	if _, err := p.LoadedModels(context.Background(), host); err != nil {
		t.Fatalf("LoadedModels returned error: %v", err)
	}
	if !stub.loadedCalled {
		t.Fatal("expected LoadedModels to call underlying provider")
	}
}

func TestProviderForHostUnsupported(t *testing.T) {
	p := New(map[string]providers.ChatProvider{
		"llama.cpp": &stubProvider{},
	})

	host := appconfig.Host{Type: "unknown", Name: "bad"}
	if _, err := p.LoadedModels(context.Background(), host); err == nil {
		t.Fatal("expected error for unsupported host type")
	}
}

func TestCloseDeduplicatesProviders(t *testing.T) {
	stub := &stubProvider{}
	p := New(map[string]providers.ChatProvider{
		"llama.cpp": stub,
		"llamacpp":  stub,
	})

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if stub.closeCalled != 1 {
		t.Fatalf("expected Close to be called once, got %d", stub.closeCalled)
	}
}
