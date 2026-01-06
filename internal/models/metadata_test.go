// internal/models/metadata_test.go
package models

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLlamaModelsVariants(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []string
	}{
		{
			name: "wrapped models",
			data: `{"models":[{"id":"m1","status":"loaded"},{"name":"m2"}]}`,
			want: []string{"m1", "m2"},
		},
		{
			name: "wrapped data",
			data: `{"data":[{"model":"m3"},{"path":"m4.gguf"}]}`,
			want: []string{"m3", "m4.gguf"},
		},
		{
			name: "direct array",
			data: `[{"id":"m5"},{"name":"m6"}]`,
			want: []string{"m5", "m6"},
		},
		{
			name: "names list",
			data: `{"models":["m7","m8"]}`,
			want: []string{"m7", "m8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := parseLlamaModels([]byte(tt.data))
			if err != nil {
				t.Fatalf("parseLlamaModels error: %v", err)
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

func TestSanitizeModelFilename(t *testing.T) {
	raw := `a/b\c:d*e?f"g<h>i|j`
	sanitized := sanitizeModelFilename(raw)
	if strings.ContainsAny(sanitized, `/\:*?"<>|`) {
		t.Fatalf("sanitizeModelFilename left invalid chars: %q", sanitized)
	}
}

func TestUniqueModelMeta(t *testing.T) {
	input := []ModelMeta{
		{Type: "llama.cpp", Name: "m1", Endpoint: "http://a"},
		{Type: "llama.cpp", Name: "m1", Endpoint: "http://a"},
		{Type: "llama.cpp", Name: "m1", Endpoint: "http://b"},
	}
	out := uniqueModelMeta(input)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique entries, got %d", len(out))
	}
}

func TestModelMetadataPath(t *testing.T) {
	path := modelMetadataPath(ModelMeta{Type: "llama.cpp", Name: "m1"})
	if !strings.Contains(path, "agonData") || !strings.Contains(path, "modelMetadata") {
		t.Fatalf("unexpected metadata path: %s", path)
	}
	if !strings.Contains(path, "llama.cpp") {
		t.Fatalf("expected model type in path, got %s", path)
	}
}

func TestRemoveTensorsMetadata(t *testing.T) {
	payload := map[string]any{
		"tensors": "drop",
		"nested": map[string]any{
			"tensors": "drop",
			"keep":    "ok",
		},
		"list": []any{
			map[string]any{"tensors": "drop"},
		},
	}
	removeTensorsMetadata(payload)
	if _, ok := payload["tensors"]; ok {
		t.Fatal("expected top-level tensors to be removed")
	}
	nested := payload["nested"].(map[string]any)
	if _, ok := nested["tensors"]; ok {
		t.Fatal("expected nested tensors to be removed")
	}
	list := payload["list"].([]any)
	item := list[0].(map[string]any)
	if _, ok := item["tensors"]; ok {
		t.Fatal("expected list tensors to be removed")
	}
}

func TestIsAlreadyLoadedResponse(t *testing.T) {
	if !isAlreadyLoadedResponse([]byte("Already Loaded")) {
		t.Fatal("expected match for already loaded response")
	}
	if isAlreadyLoadedResponse([]byte("not loaded")) {
		t.Fatal("did not expect match for unrelated response")
	}
}

func TestMetadataExists(t *testing.T) {
	model := ModelMeta{Type: "llama.cpp", Name: "unit-test-model"}
	path := modelMetadataPath(model)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	if !metadataExists(model) {
		t.Fatal("expected metadataExists to return true")
	}
}

func TestFetchLlamaModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"id":"m1"}]}`))
	}))
	defer server.Close()

	var models []ModelMeta
	found := fetchLlamaModels(server.Client(), server.URL, &models)
	if !found {
		t.Fatal("expected models to be found")
	}
	if len(models) != 1 || models[0].Name != "m1" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestIsLlamaModelLoaded(t *testing.T) {
	tests := []struct {
		name   string
		status string
		expect bool
	}{
		{name: "loaded", status: "loaded", expect: true},
		{name: "unloaded", status: "unloaded", expect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(`{"models":[{"id":"m1","status":"` + tt.status + `"}]}`))
			}))
			defer server.Close()

			loaded, err := isLlamaModelLoaded(server.Client(), &ModelMeta{
				Type:     "llama.cpp",
				Name:     "m1",
				Endpoint: server.URL,
			})
			if err != nil {
				t.Fatalf("isLlamaModelLoaded error: %v", err)
			}
			if loaded != tt.expect {
				t.Fatalf("loaded=%v, want %v", loaded, tt.expect)
			}
		})
	}
}

func TestWaitForLlamaModelLoadedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"id":"m1","status":"unloaded"}]}`))
	}))
	defer server.Close()

	err := waitForLlamaModelLoaded(server.Client(), &ModelMeta{
		Type:     "llama.cpp",
		Name:     "m1",
		Endpoint: server.URL,
	}, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
