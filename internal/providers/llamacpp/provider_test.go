// internal/providers/llamacpp/provider_test.go
package llamacpp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

func TestProviderStreamDisableStreaming(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			capturedBody = body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"role":"assistant","content":"final"}}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	req := providers.StreamRequest{
		Host:             appconfig.Host{Name: "test", URL: server.URL},
		Model:            "test-model",
		DisableStreaming: true,
		Tools: []providers.ToolDefinition{{
			Name:        "weather",
			Description: "fetches weather",
		}},
	}

	var chunks []providers.ChatMessage
	var meta providers.StreamMetadata
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			chunks = append(chunks, msg)
			return nil
		},
		OnComplete: func(m providers.StreamMetadata) error {
			meta = m
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if len(chunks) != 1 || chunks[0].Content != "final" {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
	if meta.Model != "test-model" || !meta.Done {
		t.Fatalf("unexpected metadata: %+v", meta)
	}

	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if stream, ok := payload["stream"].(bool); !ok || stream {
		t.Fatalf("expected stream=false, got %v", payload["stream"])
	}
	if toolChoice, ok := payload["tool_choice"].(string); !ok || toolChoice != "auto" {
		t.Fatalf("expected tool_choice auto, got %v", payload["tool_choice"])
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected tools in payload, got %T", payload["tools"])
	}
	toolObj, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object to be map, got %T", tools[0])
	}
	if toolType, ok := toolObj["type"].(string); !ok || toolType != "function" {
		t.Fatalf("expected tool type function, got %v", toolObj["type"])
	}
	fn, ok := toolObj["function"].(map[string]any)
	if !ok {
		t.Fatalf("expected function field to be map, got %T", toolObj["function"])
	}
	if name, ok := fn["name"].(string); !ok || name != "weather" {
		t.Fatalf("expected function name weather, got %v", fn["name"])
	}
	if desc, ok := fn["description"].(string); !ok || desc != "fetches weather" {
		t.Fatalf("expected function description fetches weather, got %v", fn["description"])
	}
}

func TestProviderStreamNoToolCapability(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"model does not support tools"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	req := providers.StreamRequest{
		Host:             appconfig.Host{Name: "test", URL: server.URL},
		Model:            "test-model",
		DisableStreaming: true,
	}

	var chunks []providers.ChatMessage
	var meta providers.StreamMetadata
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			chunks = append(chunks, msg)
			return nil
		},
		OnComplete: func(m providers.StreamMetadata) error {
			meta = m
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if len(chunks) != 1 || chunks[0].Content != "This model does not have tool capabilities." {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
	if meta.Model != "test-model" || !meta.Done {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestProviderStreamToolCallExecutes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			payload := `{"model":"test-model","choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"current_weather","arguments":"{\"location\":\"Denver\"}"}}]}}]}`
			_, _ = w.Write([]byte(payload))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	called := false
	var capturedArgs map[string]any
	req := providers.StreamRequest{
		Host:             appconfig.Host{Name: "test", URL: server.URL},
		Model:            "test-model",
		DisableStreaming: true,
		ToolExecutor: func(ctx context.Context, name string, args map[string]any) (string, error) {
			called = true
			capturedArgs = args
			if name != "current_weather" {
				t.Fatalf("unexpected tool: %s", name)
			}
			return "Sunny", nil
		},
	}

	var chunks []providers.ChatMessage
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			chunks = append(chunks, msg)
			return nil
		},
		OnComplete: func(providers.StreamMetadata) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if !called {
		t.Fatalf("expected tool executor to be called")
	}
	if loc, ok := capturedArgs["location"].(string); !ok || loc != "Denver" {
		t.Fatalf("unexpected tool args: %+v", capturedArgs)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected single chunk, got %d", len(chunks))
	}
	expected := "[Tool current_weather]\nSunny"
	if chunks[0].Content != expected {
		t.Fatalf("unexpected chunk content: %q", chunks[0].Content)
	}
}

func TestProviderStreamSSE(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("response writer does not support flushing")
			}
			_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\n"))
			flusher.Flush()
			_, _ = w.Write([]byte("data: {\"model\":\"test-model\",\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
			flusher.Flush()
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	req := providers.StreamRequest{
		Host:             appconfig.Host{Name: "test", URL: server.URL},
		Model:            "test-model",
		DisableStreaming: false,
	}

	var chunks []providers.ChatMessage
	var meta providers.StreamMetadata
	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{
		OnChunk: func(msg providers.ChatMessage) error {
			chunks = append(chunks, msg)
			return nil
		},
		OnComplete: func(m providers.StreamMetadata) error {
			meta = m
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if meta.Model != "test-model" || !meta.Done {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	var builder strings.Builder
	for _, chunk := range chunks {
		builder.WriteString(chunk.Content)
	}
	if got := builder.String(); got != "Hello world" {
		t.Fatalf("unexpected content: %q", got)
	}
}
