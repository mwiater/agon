// internal/appconfig/load_integration_test.go
package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultPathWithLlamaTypes(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	payload := `{
  "hosts": [
    { "name": "A", "url": "http://localhost:8080", "type": "llama.cpp", "models": ["m1"], "parameterTemplate": "generic" },
    { "name": "B", "url": "http://localhost:8081", "type": "llamacpp", "models": ["m2"], "parameterTemplate": "generic" }
  ]
}`
	path := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(cfg.Hosts))
	}
	if cfg.TimeoutSeconds != 600 {
		t.Fatalf("expected default timeout 600, got %d", cfg.TimeoutSeconds)
	}
}

func TestLoadLegacyFallback(t *testing.T) {
	tempDir := t.TempDir()
	payload := `{
  "hosts": [
    { "name": "A", "url": "http://localhost:8080", "type": "llama.cpp", "models": ["m1"], "parameterTemplate": "generic" }
  ]
}`
	if err := os.WriteFile(filepath.Join(tempDir, "config.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.Hosts))
	}
}

func TestLoadNoHostsError(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"hosts":[]}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	if _, err := Load(""); err == nil {
		t.Fatal("expected error for empty hosts")
	}
}

func TestLoadMissingFileError(t *testing.T) {
	tempDir := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	if _, err := Load(""); err == nil {
		t.Fatal("expected error for missing config")
	}
}
