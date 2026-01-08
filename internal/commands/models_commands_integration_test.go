// internal/commands/models_commands_integration_test.go
package agon

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
)

type routerMock struct {
	mu         sync.Mutex
	status     map[string]string
	loadCalls  []string
	unloadCalls []string
}

func newRouterMock(initial map[string]string) *routerMock {
	status := make(map[string]string, len(initial))
	for k, v := range initial {
		status[k] = v
	}
	return &routerMock{status: status}
}

func (m *routerMock) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/models":
		m.mu.Lock()
		names := make([]string, 0, len(m.status))
		for name := range m.status {
			names = append(names, name)
		}
		sort.Strings(names)
		type modelResp struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		out := struct {
			Models []modelResp `json:"models"`
		}{Models: make([]modelResp, 0, len(names))}
		for _, name := range names {
			out.Models = append(out.Models, modelResp{ID: name, Status: m.status[name]})
		}
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(out)
	case "/models/load":
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.loadCalls = append(m.loadCalls, payload.Model)
		m.status[payload.Model] = "loaded"
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case "/models/unload":
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.unloadCalls = append(m.unloadCalls, payload.Model)
		m.status[payload.Model] = "unloaded"
		m.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func withConfig(cfg *appconfig.Config, fn func()) {
	original := currentConfig
	currentConfig = cfg
	defer func() { currentConfig = original }()
	fn()
}

func TestListModelsRouter(t *testing.T) {
	mock := newRouterMock(map[string]string{
		"model-a": "loaded",
		"model-b": "unloaded",
	})
	server := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer server.Close()

	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp"},
		},
	}

	out := captureOutput(t, func() {
		withConfig(cfg, func() {
			listModelsCmd.Run(listModelsCmd, []string{})
		})
	})

	if !strings.Contains(out, "modela") || !strings.Contains(out, "modelb") {
		t.Fatalf("expected model names in output, got: %s", out)
	}
}

func TestPullModelsRouter(t *testing.T) {
	mock := newRouterMock(map[string]string{
		"model-a": "unloaded",
	})
	server := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer server.Close()

	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp", Models: []string{"model-a", "model-c"}},
		},
	}

	captureOutput(t, func() {
		withConfig(cfg, func() {
			pullModelsCmd.Run(pullModelsCmd, []string{})
		})
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.loadCalls) != 2 {
		t.Fatalf("expected 2 load calls, got %d", len(mock.loadCalls))
	}
}

func TestDeleteModelsRouter(t *testing.T) {
	mock := newRouterMock(map[string]string{
		"model-a": "loaded",
		"model-b": "unloaded",
	})
	server := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer server.Close()

	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp", Models: []string{"model-a"}},
		},
	}

	captureOutput(t, func() {
		withConfig(cfg, func() {
			deleteModelsCmd.Run(deleteModelsCmd, []string{})
		})
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.unloadCalls) != 1 || mock.unloadCalls[0] != "model-b" {
		t.Fatalf("expected unload for model-b, got %v", mock.unloadCalls)
	}
}

func TestUnloadModelsRouter(t *testing.T) {
	mock := newRouterMock(map[string]string{
		"model-a": "loaded",
		"model-b": "unloaded",
	})
	server := httptest.NewServer(http.HandlerFunc(mock.handler))
	defer server.Close()

	cfg := &appconfig.Config{
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp"},
		},
	}

	captureOutput(t, func() {
		withConfig(cfg, func() {
			unloadModelsCmd.Run(unloadModelsCmd, []string{})
		})
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.unloadCalls) != 1 || mock.unloadCalls[0] != "model-a" {
		t.Fatalf("expected unload for model-a, got %v", mock.unloadCalls)
	}
}
