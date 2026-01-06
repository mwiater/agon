// accuracy/accuracy_integration_test.go
package accuracy

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/appconfig"
)

func writePromptSuite(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "accuracy"), 0o755); err != nil {
		t.Fatalf("mkdir accuracy dir: %v", err)
	}
	suite := `{
  "system_prompt": "You are a precise facts and logic engine.",
  "tests": [
    {
      "id": 1,
      "prompt": "2+2",
      "expected_answer": 4,
      "marginOfError": 0,
      "difficulty": 1,
      "category": "math"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "accuracy", "accuracy_prompts.json"), []byte(suite), 0o644); err != nil {
		t.Fatalf("write prompt suite: %v", err)
	}
}

func readFirstResult(t *testing.T, path string) AccuracyResult {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open results: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatalf("expected result line in %s", path)
	}
	var result AccuracyResult
	if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return result
}

func TestRunAccuracyWritesJSONL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			resp := map[string]any{
				"model": "model1",
				"timings": map[string]any{
					"prompt_ms":    1.0,
					"predicted_ms": 2.0,
					"prompt_n":     1,
					"predicted_n":  1,
				},
				"choices": []map[string]any{
					{"message": map[string]any{"role": "assistant", "content": "4"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{
		AccuracyMode:   true,
		TimeoutSeconds: 5,
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp", Models: []string{"model1"}},
		},
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tempDir := t.TempDir()
	writePromptSuite(t, tempDir)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	if err := RunAccuracy(cfg); err != nil {
		t.Fatalf("RunAccuracy error: %v", err)
	}

	resultPath := filepath.Join("agonData", "modelAccuracy", "model1.jsonl")
	result := readFirstResult(t, resultPath)
	if result.Model != "model1" || result.Host != "Host01" {
		t.Fatalf("unexpected result host/model: %#v", result)
	}
	if result.Response != "4" || !result.Correct {
		t.Fatalf("expected correct response, got %#v", result)
	}
	if result.DeadlineExceeded {
		t.Fatalf("unexpected deadlineExceeded=true")
	}
}

func TestRunAccuracyDeadlineExceeded(t *testing.T) {
	tempDir := t.TempDir()
	writePromptSuite(t, tempDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/load":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/chat/completions":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("context deadline exceeded"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &appconfig.Config{
		AccuracyMode:   true,
		TimeoutSeconds: 5,
		Hosts: []appconfig.Host{
			{Name: "Host01", URL: server.URL, Type: "llama.cpp", Models: []string{"model1"}},
		},
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	if err := RunAccuracy(cfg); err != nil {
		t.Fatalf("RunAccuracy error: %v", err)
	}

	resultPath := filepath.Join("agonData", "modelAccuracy", "model1.jsonl")
	result := readFirstResult(t, resultPath)
	if !result.DeadlineExceeded {
		t.Fatalf("expected deadlineExceeded=true, got %#v", result)
	}
	if !strings.Contains(strings.ToLower(result.Response), "context deadline exceeded") {
		t.Fatalf("expected deadline error in response, got %q", result.Response)
	}
}
