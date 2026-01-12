package benchmark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mwiater/agon/internal/models"
)

func TestRunBenchmarkModelsFromMetadataValidation(t *testing.T) {
	if err := RunBenchmarkModelsFromMetadata("", "http://example"); err == nil {
		t.Fatalf("expected gpu error")
	}
	if err := RunBenchmarkModelsFromMetadata("gpu", ""); err == nil {
		t.Fatalf("expected benchmark server error")
	}
}

func TestRunBenchmarkModelsFromMetadataMatches(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
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

	metaDir := filepath.Join("agonData", "modelMetadata")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	match := models.ModelMeta{
		Type:     "llama.cpp",
		Name:     "ModelMatch",
		Endpoint: server.URL,
		GPU:      "gpu-1",
	}
	payload, err := json.Marshal(match)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "match.json"), payload, 0o644); err != nil {
		t.Fatalf("write match: %v", err)
	}

	nonMatch := models.ModelMeta{
		Type:     "llama.cpp",
		Name:     "ModelSkip",
		Endpoint: server.URL,
		GPU:      "gpu-2",
	}
	payload, err = json.Marshal(nonMatch)
	if err != nil {
		t.Fatalf("marshal non-match: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "skip.json"), payload, 0o644); err != nil {
		t.Fatalf("write non-match: %v", err)
	}

	if err := RunBenchmarkModelsFromMetadata("gpu-1", server.URL+"/"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	fileBase := Slugify(fmt.Sprintf("%s_%s", "gpu-1", "ModelMatch"))
	fileName := filepath.Join("agonData", "modelBenchmarks", fileBase+".json")
	data, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(data), `"result"`) {
		t.Fatalf("expected response persisted, got %s", string(data))
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected 1 request, got %d", got)
	}

	skipFileBase := Slugify(fmt.Sprintf("%s_%s", "gpu-2", "ModelSkip"))
	skipFile := filepath.Join("agonData", "modelBenchmarks", skipFileBase+".json")
	if _, err := os.Stat(skipFile); !os.IsNotExist(err) {
		t.Fatalf("expected skip file to not exist, got %v", err)
	}
}
