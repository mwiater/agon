package logging

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testStringer string

func (s testStringer) String() string { return string(s) }

func TestInitAndLoggingToFile(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "nested", "agon.log")

	if err := Init(logPath); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	t.Cleanup(func() {
		_ = Close()
	})

	LogEvent("hello %s", "world")
	LogMetricsEvent("metrics %s", "only")
	_ = Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "hello world") {
		t.Fatalf("expected LogEvent content, got: %s", content)
	}
	if !strings.Contains(content, "metrics only") {
		t.Fatalf("expected LogMetricsEvent content, got: %s", content)
	}
}

func TestBuildRequestMessageDefaults(t *testing.T) {
	msg := buildRequestMessage(" in ", " ", "", " tool ", map[string]any{"ok": true})
	if !strings.Contains(msg, "[IN]") {
		t.Fatalf("expected uppercased direction, got: %s", msg)
	}
	if !strings.Contains(msg, "host=unknown") {
		t.Fatalf("expected default host, got: %s", msg)
	}
	if !strings.Contains(msg, "model=unknown") {
		t.Fatalf("expected default model, got: %s", msg)
	}
	if !strings.Contains(msg, "tool=tool") {
		t.Fatalf("expected tool name, got: %s", msg)
	}
	if !strings.Contains(msg, "payload={\"ok\":true}") {
		t.Fatalf("expected payload json, got: %s", msg)
	}
}

func TestFormatPayloadVariants(t *testing.T) {
	if got := formatPayload(nil); got != "null" {
		t.Fatalf("nil payload: %s", got)
	}
	if got := formatPayload(" "); got != `""` {
		t.Fatalf("empty string payload: %s", got)
	}
	if got := formatPayload([]byte("hi")); got != "hi" {
		t.Fatalf("byte payload: %s", got)
	}
	if got := formatPayload(testStringer("ok")); got != "ok" {
		t.Fatalf("stringer payload: %s", got)
	}
}

func TestInitDiscard(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	if err := Init(""); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	LogEvent("discard")
	if buf.Len() != 0 {
		t.Fatalf("expected log output discarded, got: %s", buf.String())
	}
}
