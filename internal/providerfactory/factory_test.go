// internal/providerfactory/factory_test.go
package providerfactory

import (
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers/llamacpp"
)

func TestCollectHostTypesDefaultsToLlamaCpp(t *testing.T) {
	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{Type: ""},
			{Type: "llamacpp"},
			{Type: "llama.cpp"},
		},
	}

	types, err := collectHostTypes(cfg)
	if err != nil {
		t.Fatalf("collectHostTypes returned error: %v", err)
	}
	if len(types) != 1 || !types["llama.cpp"] {
		t.Fatalf("expected llama.cpp only, got: %#v", types)
	}
}

func TestCollectHostTypesRejectsUnsupported(t *testing.T) {
	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{{Type: "unsupported"}},
	}

	if _, err := collectHostTypes(cfg); err == nil {
		t.Fatal("expected error for unsupported host type")
	}
}

func TestNewChatProviderErrorsOnNilConfig(t *testing.T) {
	if _, err := NewChatProvider(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewChatProviderDefaultsToLlamaCpp(t *testing.T) {
	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{
				Name:   "Test",
				URL:    "http://localhost:8080",
				Type:   "",
				Models: []string{"model.gguf"},
			},
		},
	}

	provider, err := NewChatProvider(cfg)
	if err != nil {
		t.Fatalf("NewChatProvider returned error: %v", err)
	}
	if _, ok := provider.(*llamacpp.Provider); !ok {
		t.Fatalf("expected llamacpp.Provider, got %T", provider)
	}
}
