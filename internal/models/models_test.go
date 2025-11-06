// internal/models/models_test.go
package models

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestOllamaHost tests the functionality of the OllamaHost struct and its
// associated methods. It sets up a mock HTTP server to simulate the Ollama API
// and verifies that the methods for listing, pulling, deleting, and unloading
// models behave as expected. It also checks that model parameters are correctly
// retrieved and parsed.
func TestOllamaHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			if _, err := w.Write([]byte(`{"models":[{"name":"model1"},{"name":"model2"}]}`)); err != nil {
				t.Fatalf("write response for /api/tags: %v", err)
			}
		case "/api/ps":
			if _, err := w.Write([]byte(`{"models":[{"name":"model1"}]}`)); err != nil {
				t.Fatalf("write response for /api/ps: %v", err)
			}
		case "/api/show":
			if _, err := w.Write([]byte(`{"parameters":"temperature 0.8"}`)); err != nil {
				t.Fatalf("write response for /api/show: %v", err)
			}
		case "/api/pull":
			w.WriteHeader(http.StatusOK)
		case "/api/delete":
			w.WriteHeader(http.StatusOK)
		case "/api/chat":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	host := &OllamaHost{
		Name:           "Test Host",
		URL:            server.URL,
		Models:         []string{"model1", "model2"},
		client:         server.Client(),
		requestTimeout: time.Second,
	}

	if host.GetName() != "Test Host" {
		t.Errorf("Expected name 'Test Host', got '%s'", host.GetName())
	}

	if host.GetType() != "ollama" {
		t.Errorf("Expected type 'ollama', got '%s'", host.GetType())
	}

	if len(host.GetModels()) != 2 {
		t.Errorf("Expected 2 models, got %d", len(host.GetModels()))
	}

	host.PullModel("model3")
	host.DeleteModel("model1")
	host.UnloadModel("model1")

	rawModels, err := host.ListRawModels()
	if err != nil {
		t.Errorf("ListRawModels() failed: %v", err)
	}
	if len(rawModels) != 2 {
		t.Errorf("Expected 2 raw models, got %d", len(rawModels))
	}

	models, err := host.ListModels()
	if err != nil {
		t.Errorf("ListModels() failed: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	runningModels, err := host.GetRunningModels()
	if err != nil {
		t.Errorf("GetRunningModels() failed: %v", err)
	}
	if len(runningModels) != 1 {
		t.Errorf("Expected 1 running model, got %d", len(runningModels))
	}

	params, err := host.GetModelParameters()
	if err != nil {
		t.Errorf("GetModelParameters() failed: %v", err)
	}
	if len(params) != 2 {
		t.Errorf("Expected 2 sets of parameters, got %d", len(params))
	}

	modelParams, err := host.getModelParametersFromAPI("model1")
	if err != nil {
		t.Errorf("getModelParametersFromAPI() failed: %v", err)
	}
	if !strings.Contains(modelParams.Parameters, "temperature 0.8") {
		t.Errorf("Expected parameters to contain 'temperature 0.8', got '%s'", modelParams.Parameters)
	}
}

// TestExtractSettings tests the extractSettings function to ensure it correctly
// parses a string of model parameters and extracts the key-value pairs. It
// verifies that parameters with and without explicit values are handled correctly,
// and that leading/trailing whitespace is properly trimmed.
func TestExtractSettings(t *testing.T) {
	paramsText := `
		temperature 0.8
		parameter top_p 0.9
		top_k=40
		repeat_penalty = 1.1
		min_p
	`
	settings := extractSettings(paramsText)

	if settings["temperature"] != "0.8" {
		t.Errorf("Expected temperature to be '0.8', got '%s'", settings["temperature"])
	}
	if settings["top_p"] != "0.9" {
		t.Errorf("Expected top_p to be '0.9', got '%s'", settings["top_p"])
	}
	if settings["top_k"] != "40" {
		t.Errorf("Expected top_k to be '40', got '%s'", settings["top_k"])
	}
	if settings["repeat_penalty"] != "1.1" {
		t.Errorf("Expected repeat_penalty to be '1.1', got '%s'", settings["repeat_penalty"])
	}
	if settings["min_p"] != "n/a" {
		t.Errorf("Expected min_p to be 'n/a', got '%s'", settings["min_p"])
	}
}
