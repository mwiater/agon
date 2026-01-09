// internal/commands/run_accuracy.go
package agon

import (
	"github.com/mwiater/agon/internal/accuracy"
	"github.com/spf13/cobra"
)

// runAccuracyCmd implements 'run accuracy', which batches accuracy-related workflows.
var runAccuracyCmd = &cobra.Command{
	Use:   "accuracy",
	Short: "Run accuracy batch workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		return accuracy.RunAccuracyBatch(runAccuracyOpts.ParameterTemplate)
	},
}

func init() {
	runCmd.AddCommand(runAccuracyCmd)
	runAccuracyCmd.Flags().StringVar(&runAccuracyOpts.ParameterTemplate, "parameterTemplate", "accuracy", "Parameter template to apply (accuracy|generic|fact_checker|creative)")
}

var runAccuracyOpts struct {
	ParameterTemplate string
}
