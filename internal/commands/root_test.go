// internal/commands/root_test.go
package agon

import (
	"bytes"
	"strings"
	"testing"
)

// TestRootCmd verifies running the root command with an invalid subcommand reports an error.
func TestRootCmd(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetErr(b)

	rootCmd.SetArgs([]string{"nonexistent"})
	_, err := rootCmd.ExecuteC()

	if err == nil {
		t.Error("Expected an error for a nonexistent command, but got none")
	}

	expected := "unknown command \"nonexistent\" for \"agon\""
	if !strings.Contains(b.String(), expected) {
		t.Errorf("Expected output to contain '%s', but got '%s'", expected, b.String())
	}
}
