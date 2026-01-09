// internal/commands/benchmark_accuracy.go
package agon

import (
	"github.com/mwiater/agon/internal/accuracy"
	"github.com/spf13/cobra"
)

// benchmarkAccuracyCmd implements 'benchmark accuracy', which batches accuracy-related workflows.
var benchmarkAccuracyCmd = &cobra.Command{
	Use:   "accuracy",
	Short: "Run accuracy batch workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		return accuracy.RunAccuracyBatch(benchmarkAccuracyOpts.ParameterTemplate)
	},
}

func init() {
	benchmarkCmd.AddCommand(benchmarkAccuracyCmd)
	benchmarkAccuracyCmd.Flags().StringVar(&benchmarkAccuracyOpts.ParameterTemplate, "parameterTemplate", "accuracy", "Parameter template to apply (accuracy|generic|fact_checker|creative)")
}

var benchmarkAccuracyOpts struct {
	ParameterTemplate string
}
