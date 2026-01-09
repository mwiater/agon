// internal/appconfig/appconfig_test.go
package appconfig

import (
	"os"
	"testing"
	"time"
)

// TestLoad tests the Load function to ensure it correctly handles various
// scenarios, including valid and invalid configurations. It verifies that a
// valid configuration file is loaded without error, while files with invalid
// JSON, no hosts, or that are nonexistent result in an appropriate error. This
// test uses temporary files to simulate different configuration scenarios and
// asserts that the function behaves as expected in each case.
func TestLoad(t *testing.T) {
	validConfig := `{
        "hosts": [
            {
                "name": "Test Host",
                "url": "http://localhost:8080",
                "type": "llama.cpp",
                "models": ["model1", "model2"],
                "parameterTemplate": "generic"
            }
        ]
    }`
	tmpfile, err := os.CreateTemp("", "config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write([]byte(validConfig)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Load() with valid config failed: %v", err)
	}
	if len(cfg.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.Hosts))
	}

	if cfg.TimeoutSeconds != 600 {
		t.Fatalf("expected default timeout of 600 seconds, got %d", cfg.TimeoutSeconds)
	}

	if cfg.RequestTimeout() != 600*time.Second {
		t.Fatalf("expected default request timeout of 600s, got %v", cfg.RequestTimeout())
	}

	if cfg.MCPInitTimeoutDuration() != 10*time.Second {
		t.Fatalf("expected default MCP init timeout of 10s, got %v", cfg.MCPInitTimeoutDuration())
	}

	if cfg.MCPRetryAttempts() != 1 {
		t.Fatalf("expected default MCP retry attempts of 1, got %d", cfg.MCPRetryAttempts())
	}

	invalidJSON := `{ "hosts": [`
	tmpfile2, err := os.CreateTemp("", "config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile2.Name())
	if _, err := tmpfile2.Write([]byte(invalidJSON)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile2.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(tmpfile2.Name()); err == nil {
		t.Fatal("Load() with invalid JSON should have failed")
	}

	noHosts := `{ "hosts": [] }`
	tmpfile3, err := os.CreateTemp("", "config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile3.Name())
	if _, err := tmpfile3.Write([]byte(noHosts)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile3.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(tmpfile3.Name()); err == nil {
		t.Fatal("Load() with no hosts should have failed")
	}

	if _, err := Load("nonexistent.json"); err == nil {
		t.Fatal("Load() with nonexistent file should have failed")
	}
}
