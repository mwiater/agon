package benchmark

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
)

func TestBenchmarkModelsValidationErrors(t *testing.T) {
	cfg := &appconfig.Config{Hosts: []appconfig.Host{{Name: "one", Models: []string{"m1"}}}}
	if err := BenchmarkModels(cfg); err == nil {
		t.Fatalf("expected error when fewer than two hosts")
	}

	cfg = &appconfig.Config{
		Hosts: []appconfig.Host{
			{Name: "one", Models: []string{"m1", "m2"}},
			{Name: "two", Models: []string{"m3"}},
		},
	}
	if err := BenchmarkModels(cfg); err == nil {
		t.Fatalf("expected error when a host has multiple models")
	}
}

func TestRunBenchmarkModelsNilConfig(t *testing.T) {
	if err := RunBenchmarkModels(nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunBenchmarkModelValidation(t *testing.T) {
	if err := RunBenchmarkModel("", "gpu", "http://example"); err == nil {
		t.Fatalf("expected model name error")
	}
	if err := RunBenchmarkModel("model", "", "http://example"); err == nil {
		t.Fatalf("expected gpu name error")
	}
	if err := RunBenchmarkModel("model", "gpu", ""); err == nil {
		t.Fatalf("expected endpoint error")
	}
}

func TestRunBenchmarkModelSkipsExisting(t *testing.T) {
	tempDir := t.TempDir()
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	modelName := "TestModel"
	gpuName := "GPU1"
	fileBase := Slugify(fmt.Sprintf("%s_%s", gpuName, modelName))
	fileName := filepath.Join("agonData", "modelBenchmarks", fileBase+".json")

	if err := os.MkdirAll(filepath.Dir(fileName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(fileName, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := RunBenchmarkModel(modelName, gpuName, "http://example"); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestRunBenchmarkModelHTTPFlow(t *testing.T) {
	tempDir := t.TempDir()
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()

	modelName := "TestModel"
	gpuName := "GPU1"
	if err := RunBenchmarkModel(modelName, gpuName, server.URL); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	fileBase := Slugify(fmt.Sprintf("%s_%s", gpuName, modelName))
	fileName := filepath.Join("agonData", "modelBenchmarks", fileBase+".json")
	data, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(data), `"result"`) {
		t.Fatalf("expected response persisted, got %s", string(data))
	}
}

func TestRunBenchmarkModelHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad request`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	prevDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	err = RunBenchmarkModel("Model", "GPU", server.URL)
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status error, got %v", err)
	}
}
