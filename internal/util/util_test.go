// internal/util/util_test.go
package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	data := []byte("test payload")

	if err := WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("unexpected file contents: got %q want %q", got, data)
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "no truncation", in: "hello", max: 10, want: "hello"},
		{name: "ascii truncation", in: "helloworld", max: 5, want: "hello…"},
		{name: "multibyte truncation", in: "こんにちは世界", max: 4, want: "こんにち…"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TruncateRunes(tt.in, tt.max); got != tt.want {
				t.Fatalf("TruncateRunes(%q,%d)=%q want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

func TestTruncateToWidth(t *testing.T) {
	t.Parallel()

	input := "line1\nSecondLine"
	want := "line1\nSecon…"

	if got := TruncateToWidth(input, 5); got != want {
		t.Fatalf("TruncateToWidth result mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestWrapToWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{
			name:  "wrap words",
			text:  "one two three four",
			width: 10,
			want:  "one two\nthree four",
		},
		{
			name:  "long word split",
			text:  "supercalifragilisticexpialidocious",
			width: 5,
			want: strings.Join([]string{
				"super",
				"calif",
				"ragil",
				"istic",
				"expia",
				"lidoc",
				"ious",
			}, "\n"),
		},
		{
			name:  "preserve blank lines",
			text:  "para one\n\npara two",
			width: 20,
			want:  "para one\n\npara two",
		},
		{
			name:  "non-positive width no-op",
			text:  "no wrap",
			width: 0,
			want:  "no wrap",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WrapToWidth(tt.text, tt.width); got != tt.want {
				t.Fatalf("WrapToWidth(%q,%d)=%q want %q", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func TestMinMax(t *testing.T) {
	t.Parallel()

	if got := Min(3, 7); got != 3 {
		t.Fatalf("Min(3,7)=%d want 3", got)
	}
	if got := Min(9, -1); got != -1 {
		t.Fatalf("Min(9,-1)=%d want -1", got)
	}
	if got := Max(3, 7); got != 7 {
		t.Fatalf("Max(3,7)=%d want 7", got)
	}
	if got := Max(9, -1); got != 9 {
		t.Fatalf("Max(9,-1)=%d want 9", got)
	}
}

func TestBoolToInt(t *testing.T) {
	t.Parallel()

	if BoolToInt(true) != 1 {
		t.Fatalf("BoolToInt(true) != 1")
	}
	if BoolToInt(false) != 0 {
		t.Fatalf("BoolToInt(false) != 0")
	}
}
