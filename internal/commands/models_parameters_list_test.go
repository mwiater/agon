// internal/commands/models_parameters_list_test.go
package agon

import (
	"bytes"
	"testing"
)

// TestListModelParametersCmd ensures the command executes without errors.
func TestListModelParametersCmd(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)

	listModelParametersCmd.Run(listModelParametersCmd, []string{})

}
