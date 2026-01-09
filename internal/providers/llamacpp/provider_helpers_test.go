// internal/providers/llamacpp/provider_helpers_test.go
package llamacpp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

func TestParseModelsVariants(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []string
	}{
		{name: "wrapped models", data: `{"models":[{"id":"m1"},{"name":"m2"}]}`, want: []string{"m1", "m2"}},
		{name: "wrapped data", data: `{"data":[{"model":"m3"},{"path":"m4.gguf"}]}`, want: []string{"m3", "m4.gguf"}},
		{name: "direct array", data: `[{"id":"m5"},{"name":"m6"}]`, want: []string{"m5", "m6"}},
		{name: "names list", data: `{"models":["m7","m8"]}`, want: []string{"m7", "m8"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := parseModels([]byte(tt.data))
			if err != nil {
				t.Fatalf("parseModels error: %v", err)
			}
			if len(models) != len(tt.want) {
				t.Fatalf("expected %d models, got %d", len(tt.want), len(models))
			}
			for i, want := range tt.want {
				if got := modelDisplayName(models[i]); got != want {
					t.Fatalf("model %d name = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestStatusFieldUnmarshalJSON(t *testing.T) {
	var s statusField
	if err := s.UnmarshalJSON([]byte(`"loaded"`)); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if s.Value != "loaded" {
		t.Fatalf("expected loaded, got %q", s.Value)
	}
	if err := s.UnmarshalJSON([]byte(`{"value":"unloaded"}`)); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if s.Value != "unloaded" {
		t.Fatalf("expected unloaded, got %q", s.Value)
	}
	if err := s.UnmarshalJSON([]byte(`null`)); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if s.Value != "" {
		t.Fatalf("expected empty value, got %q", s.Value)
	}
}

func TestMsToNs(t *testing.T) {
	if got := msToNs(-1); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := msToNs(1.5); got != 1500000 {
		t.Fatalf("expected 1500000, got %d", got)
	}
}

func TestIsAlreadyLoadedError(t *testing.T) {
	body := []byte(`{"error":{"message":"model already loaded"}}`)
	if !isAlreadyLoadedError(400, body) {
		t.Fatal("expected already loaded match")
	}
	if isAlreadyLoadedError(500, body) {
		t.Fatal("expected false for non-400 status")
	}
}

func TestApplyParameters(t *testing.T) {
	topK := 42
	temp := 0.7
	nProbs := 5
	params := appconfig.LlamaParams{TopK: &topK, Temperature: &temp, NProbs: &nProbs}
	payload := map[string]any{}
	applyParameters(payload, params)
	if payload["top_k"] != 42 {
		t.Fatalf("expected top_k to be set")
	}
	if payload["temperature"] != 0.7 {
		t.Fatalf("expected temperature to be set")
	}
	if payload["n_probs"] != 5 {
		t.Fatalf("expected n_probs to be set")
	}
}

func TestNormalizeToolArgsWeather(t *testing.T) {
	args := map[string]any{"city": "Seattle", "state": "WA"}
	normalized := normalizeToolArgs("current_weather", args, nil)
	if normalized["location"] != "Seattle, WA" {
		t.Fatalf("expected location to be composed, got %#v", normalized["location"])
	}
}

func TestParseToolArguments(t *testing.T) {
	raw := json.RawMessage(`{"key":"val"}`)
	args, err := parseToolArguments(raw)
	if err != nil {
		t.Fatalf("parseToolArguments error: %v", err)
	}
	if args["key"] != "val" {
		t.Fatalf("expected key=val, got %#v", args)
	}

	rawString := json.RawMessage(`"{\"k\":\"v\"}"`)
	args, err = parseToolArguments(rawString)
	if err != nil {
		t.Fatalf("parseToolArguments string error: %v", err)
	}
	if args["k"] != "v" {
		t.Fatalf("expected k=v, got %#v", args)
	}
}

func TestIsNoToolCapabilityResponse(t *testing.T) {
	if !isNoToolCapabilityResponse([]byte("tool support disabled")) {
		t.Fatal("expected tool capability detection")
	}
	payload := []byte(`{"error":"tool capability missing"}`)
	if !isNoToolCapabilityResponse(payload) {
		t.Fatal("expected tool capability detection in json")
	}
	if isNoToolCapabilityResponse([]byte("all good")) {
		t.Fatal("unexpected tool detection")
	}
}

func TestSanitizeMessages(t *testing.T) {
	in := []providers.ChatMessage{
		{Role: "", Content: "hi"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: " "},
	}
	out := sanitizeMessages(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	if out[0].Role != "user" {
		t.Fatalf("expected default role user")
	}
}

func TestToOpenAIMessages(t *testing.T) {
	in := []providers.ChatMessage{{Role: " user ", Content: " hi "}}
	out := toOpenAIMessages(in)
	if len(out) != 1 || out[0].Role != "user" || out[0].Content != "hi" {
		t.Fatalf("unexpected openai messages: %#v", out)
	}
}

func TestHostIdentifier(t *testing.T) {
	if got := hostIdentifier(appconfig.Host{Name: "A", URL: "http://x"}); got != "A" {
		t.Fatalf("expected name, got %q", got)
	}
	if got := hostIdentifier(appconfig.Host{URL: "http://x"}); got != "http://x" {
		t.Fatalf("expected url, got %q", got)
	}
	if got := hostIdentifier(appconfig.Host{}); got != "llama.cpp-host" {
		t.Fatalf("expected default host, got %q", got)
	}
}

func TestModelStatusValue(t *testing.T) {
	model := llamaModel{Status: statusField{Value: " loaded "}}
	if got := modelStatusValue(model); got != "loaded" {
		t.Fatalf("expected loaded, got %q", got)
	}
}

func TestNormalizeToolArgsNoToolName(t *testing.T) {
	args := map[string]any{"city": "Paris"}
	tools := []providers.ToolDefinition{{Name: "current_weather"}}
	normalized := normalizeToolArgs("", args, tools)
	if _, ok := normalized["location"]; !ok {
		t.Fatalf("expected location to be added, got %#v", normalized)
	}
}

func TestParseToolArgumentsEmpty(t *testing.T) {
	args, err := parseToolArguments(json.RawMessage(`""`))
	if err != nil {
		t.Fatalf("parseToolArguments empty error: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected empty map, got %#v", args)
	}
}

func TestIsAlreadyLoadedErrorText(t *testing.T) {
	body := []byte("Model already loaded")
	if !isAlreadyLoadedError(400, body) {
		t.Fatal("expected already loaded match")
	}
	if isAlreadyLoadedError(400, []byte("other")) {
		t.Fatal("unexpected match for non loaded message")
	}
}

func TestSanitizeMessagesDropsEmptyNonAssistant(t *testing.T) {
	out := sanitizeMessages([]providers.ChatMessage{{Role: "user", Content: ""}})
	if len(out) != 0 {
		t.Fatalf("expected empty output, got %#v", out)
	}
}

func TestExecuteToolCallsWithoutExecutor(t *testing.T) {
	call := toolCall{}
	call.Function.Name = "current_time"
	call.Function.Arguments = json.RawMessage(`{"timezone":"UTC"}`)
	req := providers.StreamRequest{}
	out, err := executeToolCalls(nil, req, []toolCall{call})
	if err != nil {
		t.Fatalf("executeToolCalls error: %v", err)
	}
	if !strings.Contains(out, "current_time") {
		t.Fatalf("unexpected output: %s", out)
	}
}
