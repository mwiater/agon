// servers/benchmark/main_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildArgsDefaults(t *testing.T) {
	args, err := buildArgs(BenchRequest{Model: "model.gguf"})
	if err != nil {
		t.Fatalf("buildArgs error: %v", err)
	}
	want := []string{"-m", "model.gguf", "-ngl", "99", "-fa", "1", "--no-host", "1", "-t", "8", "-ub", "512", "-o", "json"}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d", len(want), len(args))
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildArgsOverrides(t *testing.T) {
	ngl := 7
	fa := 0
	noHost := 0
	threads := 2
	ubatch := 128
	req := BenchRequest{
		Model:    "model.gguf",
		NGL:      &ngl,
		FlashAttn: &fa,
		NoHost:   &noHost,
		Threads:  &threads,
		UBatch:   &ubatch,
	}
	args, err := buildArgs(req)
	if err != nil {
		t.Fatalf("buildArgs error: %v", err)
	}
	if strings.Join(args, " ") == "" {
		t.Fatal("expected non-empty args")
	}
}

func TestBuildArgsRejectsInvalid(t *testing.T) {
	bad := -1
	if _, err := buildArgs(BenchRequest{Model: "m", NGL: &bad}); err == nil {
		t.Fatal("expected error for invalid ngl")
	}
}

func TestItoa(t *testing.T) {
	if got := itoa(0); got != "0" {
		t.Fatalf("itoa(0) = %q, want %q", got, "0")
	}
	if got := itoa(12); got != "12" {
		t.Fatalf("itoa(12) = %q, want %q", got, "12")
	}
	if got := itoa(-5); got != "-5" {
		t.Fatalf("itoa(-5) = %q, want %q", got, "-5")
	}
}

func TestDecodeJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/benchmark", strings.NewReader(`{"model":"m1"}`))
	rec := httptest.NewRecorder()
	var out BenchRequest
	if err := decodeJSON(rec, req, &out, 1024); err != nil {
		t.Fatalf("decodeJSON error: %v", err)
	}
	if out.Model != "m1" {
		t.Fatalf("expected model m1, got %q", out.Model)
	}
}

func TestDecodeJSONRejectsUnknownField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/benchmark", strings.NewReader(`{"model":"m1","extra":1}`))
	rec := httptest.NewRecorder()
	var out BenchRequest
	if err := decodeJSON(rec, req, &out, 1024); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestDecodeJSONRejectsNilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/benchmark", nil)
	req.Body = nil
	rec := httptest.NewRecorder()
	var out BenchRequest
	if err := decodeJSON(rec, req, &out, 1024); err == nil {
		t.Fatal("expected error for nil body")
	}
}
