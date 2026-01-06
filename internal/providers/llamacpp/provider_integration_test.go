// internal/providers/llamacpp/provider_integration_test.go
package llamacpp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

func TestStreamNonStreamingJSONMode(t *testing.T) {
	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			resp := map[string]any{
				"model": "test",
				"timings": map[string]any{
					"prompt_ms":     1.0,
					"predicted_ms":  2.0,
					"prompt_n":      1,
					"predicted_n":   2,
					"cache_n":       0,
					"prompt_per_second":  0.0,
					"predicted_per_second": 0.0,
					"prompt_per_token_ms": 0.0,
					"predicted_per_token_ms": 0.0,
				},
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": "ok"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	req := providers.StreamRequest{
		Host:             appconfig.Host{URL: server.URL},
		Model:            "test",
		History:          []providers.ChatMessage{{Role: "user", Content: "hi"}},
		JSONMode:         true,
		DisableStreaming: true,
	}

	var gotChunk string
	var gotComplete bool
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			gotChunk = msg.Content
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error {
			gotComplete = meta.Done
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if gotChunk != "ok" {
		t.Fatalf("expected chunk ok, got %q", gotChunk)
	}
	if !gotComplete {
		t.Fatalf("expected completion")
	}
	if captured == nil {
		t.Fatalf("expected request payload")
	}
	if _, ok := captured["response_format"]; !ok {
		t.Fatalf("expected response_format in payload, got %#v", captured)
	}
}

func TestStreamToolCallSummaryWithoutExecutor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			resp := map[string]any{
				"model": "test",
				"timings": map[string]any{
					"prompt_ms":    1.0,
					"predicted_ms": 1.0,
					"prompt_n":     1,
					"predicted_n":  1,
				},
				"choices": []map[string]any{
					{"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{"type": "function", "function": map[string]any{"name": "current_time", "arguments": `{"timezone":"UTC"}`}},
						},
					}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := New(&appconfig.Config{TimeoutSeconds: 5})
	req := providers.StreamRequest{
		Host:             appconfig.Host{URL: server.URL},
		Model:            "test",
		History:          []providers.ChatMessage{{Role: "user", Content: "hi"}},
		DisableStreaming: true,
	}

	var gotChunk string
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			gotChunk = msg.Content
			return nil
		},
		OnComplete: func(meta providers.StreamMetadata) error { return nil },
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	if gotChunk == "" {
		t.Fatalf("expected tool summary chunk")
	}
	if !strings.Contains(gotChunk, "current_time") {
		t.Fatalf("expected tool name in chunk, got %q", gotChunk)
	}
}
