// internal/models/llama_host_test.go
package models

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLlamaCppHostListModelsVariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		body       string
		wantIDs    []string
		wantStatus map[string]string
	}{
		{
			name:       "wrapped data",
			body:       `{"data":[{"id":"model-a","status":"loaded"},{"name":"model-b","status":"unloaded"}]}`,
			wantIDs:    []string{"model-a", "model-b"},
			wantStatus: map[string]string{"model-a": "loaded", "model-b": "unloaded"},
		},
		{
			name:       "wrapped models",
			body:       `{"models":[{"name":"model-c","status":{"value":"loading"}}]}`,
			wantIDs:    []string{"model-c"},
			wantStatus: map[string]string{"model-c": "loading"},
		},
		{
			name:       "direct array",
			body:       `[{"id":"model-d","status":"loaded"}]`,
			wantIDs:    []string{"model-d"},
			wantStatus: map[string]string{"model-d": "loaded"},
		},
		{
			name:       "names list",
			body:       `{"models":["model-e","model-f"]}`,
			wantIDs:    []string{"model-e", "model-f"},
			wantStatus: map[string]string{"model-e": "", "model-f": ""},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			host := &LlamaCppHost{
				Name:           "test",
				URL:            server.URL,
				Models:         []string{},
				client:         server.Client(),
				requestTimeout: time.Second,
			}

			models, err := host.listModels()
			if err != nil {
				t.Fatalf("listModels returned error: %v", err)
			}
			if len(models) != len(tc.wantIDs) {
				t.Fatalf("expected %d models, got %d", len(tc.wantIDs), len(models))
			}

			for i, want := range tc.wantIDs {
				got := modelDisplayName(models[i])
				if got != want {
					t.Fatalf("expected model %q, got %q", want, got)
				}
				status := modelStatusValue(models[i])
				if status != tc.wantStatus[want] {
					t.Fatalf("expected status %q for %q, got %q", tc.wantStatus[want], want, status)
				}
			}
		})
	}
}

func TestLlamaCppHostGetRunningModels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a","status":"loaded"},{"id":"model-b","status":"unloaded"}]}`))
	}))
	defer server.Close()

	host := &LlamaCppHost{
		Name:           "test",
		URL:            server.URL,
		Models:         []string{},
		client:         server.Client(),
		requestTimeout: time.Second,
	}

	running, err := host.GetRunningModels()
	if err != nil {
		t.Fatalf("GetRunningModels returned error: %v", err)
	}
	if _, ok := running["model-a"]; !ok {
		t.Fatalf("expected model-a to be running")
	}
	if _, ok := running["model-b"]; ok {
		t.Fatalf("expected model-b to be unloaded")
	}
}

func TestLlamaCppHostUnloadModelRequest(t *testing.T) {
	t.Parallel()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/unload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		captured = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	host := &LlamaCppHost{
		Name:           "test",
		URL:            server.URL,
		Models:         []string{},
		client:         server.Client(),
		requestTimeout: time.Second,
	}

	host.UnloadModel("model-a")

	if string(captured) == "" {
		t.Fatalf("expected unload request body")
	}
	if string(captured) != `{"model":"model-a"}` {
		t.Fatalf("unexpected body: %s", string(captured))
	}
}
