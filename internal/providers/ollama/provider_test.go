// internal/providers/ollama/provider_test.go
package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

// TestProviderStreamDisableStreaming verifies that when streaming is disabled, the provider
// makes a single request and correctly processes the non-streaming response.
func TestProviderStreamDisableStreaming(t *testing.T) {
	t.Parallel()

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"test-model","message":{"role":"assistant","content":"final"},"done":true,"total_duration":123}`))
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	host := appconfig.Host{Name: "test", URL: server.URL}
	req := providers.StreamRequest{
		Host:             host,
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

// TestProviderStreamNoToolCapability tests the provider's handling of a response
// indicating the model does not support tools.
func TestProviderStreamNoToolCapability(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"model does not support tools"}`))
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	host := appconfig.Host{Name: "test", URL: server.URL}
	req := providers.StreamRequest{
		Host:             host,
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

// TestIsNoToolCapabilityResponse checks the helper function that determines if a
// response body indicates a lack of tool support.
func TestIsNoToolCapabilityResponse(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want bool
	}{
		{name: "json error", body: []byte(`{"error":"model does not support tools"}`), want: true},
		{name: "plain text", body: []byte("tools capability missing"), want: true},
		{name: "unrelated", body: []byte("some other error"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoToolCapabilityResponse(tc.body); got != tc.want {
				t.Fatalf("expected %v got %v", tc.want, got)
			}
		})
	}
}

// TestProviderStreamToolCallExecutes ensures that a tool call in the response
// triggers the provided tool executor function.
func TestProviderStreamToolCallExecutes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payload := `{"model":"test-model","message":{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"current_weather","arguments":"{\"location\":\"Denver\"}"}}]},"done":true}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	host := appconfig.Host{Name: "test", URL: server.URL}
	called := false
	var capturedArgs map[string]any

	req := providers.StreamRequest{
		Host:             host,
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

// TestProviderStreamToolCallObjectArgs tests the provider's ability to handle tool
// call arguments provided as a JSON object.
func TestProviderStreamToolCallObjectArgs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payload := `{"model":"test-model","message":{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"current_weather","arguments":{"location":"Boulder"}}}]},"done":true}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	host := appconfig.Host{Name: "test", URL: server.URL}
	called := false
	var capturedArgs map[string]any

	req := providers.StreamRequest{
		Host:             host,
		Model:            "test-model",
		DisableStreaming: true,
		ToolExecutor: func(ctx context.Context, name string, args map[string]any) (string, error) {
			called = true
			capturedArgs = args
			if name != "current_weather" {
				t.Fatalf("unexpected tool: %s", name)
			}
			return "Cloudy", nil
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
	if loc, ok := capturedArgs["location"].(string); !ok || loc != "Boulder" {
		t.Fatalf("unexpected tool args: %+v", capturedArgs)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected single chunk, got %d", len(chunks))
	}
	expected := "[Tool current_weather]\nCloudy"
	if chunks[0].Content != expected {
		t.Fatalf("unexpected chunk content: %q", chunks[0].Content)
	}
}

// TestProviderStreamToolCallCityCountry verifies that the provider can synthesize
// a 'location' argument from 'city' and 'country' for weather tools.
func TestProviderStreamToolCallCityCountry(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		payload := `{"model":"test-model","message":{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"current_weather","arguments":{"city":"Houston","country":"United States"}}}]},"done":true}`
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	cfg := &appconfig.Config{TimeoutSeconds: 5}
	provider := New(cfg)

	host := appconfig.Host{Name: "test", URL: server.URL}
	called := false
	var capturedArgs map[string]any

	req := providers.StreamRequest{
		Host:             host,
		Model:            "test-model",
		DisableStreaming: true,
		ToolExecutor: func(ctx context.Context, name string, args map[string]any) (string, error) {
			called = true
			capturedArgs = args
			if name != "current_weather" {
				t.Fatalf("unexpected tool: %s", name)
			}
			return "Rainy", nil
		},
	}

	err := provider.Stream(context.Background(), req, providers.StreamCallbacks{})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if !called {
		t.Fatalf("expected tool executor to be called")
	}
	loc, ok := capturedArgs["location"].(string)
	if !ok || loc != "Houston, United States" {
		t.Fatalf("expected location to be synthesized, got %+v", capturedArgs)
	}
}

// TestProviderStreamLegacyToolCallMarkup tests parsing of legacy tool call markup.
func TestProviderStreamLegacyToolCallMarkup(t *testing.T) {
	t.Parallel()

	content := `<tool_call>[{"arguments":{"location":"Portland, OR"}}]</tool_call>`
	tools := []providers.ToolDefinition{{ 
		Name:        "current_weather",
		Description: "fetches weather",
	}}

	calls, cleaned := parseLegacyToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected single tool call, got %d", len(calls))
	}
	if cleaned != "" {
		t.Fatalf("expected cleaned content to be empty, got %q", cleaned)
	}
	if calls[0].Function.Name != "current_weather" {
		t.Fatalf("unexpected tool name: %q", calls[0].Function.Name)
	}

	args, err := parseToolArguments(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("parseToolArguments returned error: %v", err)
	}
	normalized := normalizeToolArgs(calls[0].Function.Name, args, tools)
	loc, ok := normalized["location"].(string)
	if !ok || loc != "Portland, OR" {
		t.Fatalf("expected location to be preserved, got %+v", normalized)
	}
}

// TestProviderStreamLegacyToolCallSingleQuoteArgs tests parsing of legacy tool calls
// where arguments are enclosed in single quotes.
func TestProviderStreamLegacyToolCallSingleQuoteArgs(t *testing.T) {
	t.Parallel()

	content := `<tool_call>[{"function":"weather","parameters":"{ 'city': 'Portland', 'country': 'USA' }"}]</tool_call>`
	tools := []providers.ToolDefinition{{ 
		Name:        "current_weather",
		Description: "fetches weather",
	}}

	calls, cleaned := parseLegacyToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected single tool call, got %d", len(calls))
	}
	if cleaned != "" {
		t.Fatalf("expected cleaned content to be empty, got %q", cleaned)
	}

	call := calls[0]
	if call.Function.Name != "current_weather" {
		t.Fatalf("unexpected tool name: %q", call.Function.Name)
	}
	args, err := parseToolArguments(call.Function.Arguments)
	if err != nil {
		t.Fatalf("parseToolArguments returned error: %v", err)
	}
	normalized := normalizeToolArgs(call.Function.Name, args, tools)
	loc, ok := normalized["location"].(string)
	if !ok || loc != "Portland, USA" {
		t.Fatalf("expected synthesized location, got %+v", normalized)
	}
}

// TestParseLegacyToolCallMarkupBareArguments tests parsing of legacy tool calls
// with bare or unusual argument formats.
func TestParseLegacyToolCallMarkupBareArguments(t *testing.T) {
	t.Parallel()

	content := `<tool_call>[{"arguments":{"none"},"name":"get_time"}]</tool_call>`
	tools := []providers.ToolDefinition{{ 
		Name:        "current_time",
		Description: "reports time",
		Parameters:  map[string]any{"type": "object"},
	}}

	calls, cleaned := parseLegacyToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected single tool call, got %d", len(calls))
	}
	if cleaned != "" {
		t.Fatalf("expected cleaned content to be empty, got %q", cleaned)
	}

	args, err := parseToolArguments(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("parseToolArguments returned error: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected empty arguments, got %+v", args)
	}
	if calls[0].Function.Name != "current_time" {
		t.Fatalf("unexpected tool name: %q", calls[0].Function.Name)
	}
}

// TestParseLegacyToolCallMarkupDuplicateArguments tests robustness against duplicate
// 'arguments' keys in legacy tool call markup.
func TestParseLegacyToolCallMarkupDuplicateArguments(t *testing.T) {
	t.Parallel()

	content := `<tool_call>[{"arguments":{"none"},"name":"datetime","arguments":{"none"}}]</tool_call>`
	tools := []providers.ToolDefinition{{ 
		Name:        "current_time",
		Description: "reports time",
		Parameters:  map[string]any{"type": "object"},
	}}

	calls, cleaned := parseLegacyToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected single tool call, got %d", len(calls))
	}
	if cleaned != "" {
		t.Fatalf("expected cleaned content to be empty, got %q", cleaned)
	}

	args, err := parseToolArguments(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("parseToolArguments returned error: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected empty arguments, got %+v", args)
	}
	if calls[0].Function.Name != "current_time" {
		t.Fatalf("unexpected tool name: %q", calls[0].Function.Name)
	}
}

// TestParseLegacyToolCallMarkupTrailingBrace tests robustness against trailing
// braces in legacy tool call markup.
func TestParseLegacyToolCallMarkupTrailingBrace(t *testing.T) {
	t.Parallel()

	content := `<tool_call>[{"arguments":{"none"},"name":"None"}}]</tool_call>`
	tools := []providers.ToolDefinition{{ 
		Name:        "current_time",
		Description: "reports time",
		Parameters:  map[string]any{"type": "object"},
	}}

	calls, cleaned := parseLegacyToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected single tool call, got %d", len(calls))
	}
	if cleaned != "" {
		t.Fatalf("expected cleaned content to be empty, got %q", cleaned)
	}

	args, err := parseToolArguments(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("parseToolArguments returned error: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected empty arguments, got %+v", args)
	}
}
