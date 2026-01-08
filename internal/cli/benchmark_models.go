package agon

import (
	"github.com/mwiater/agon/internal/benchmark"
	"github.com/spf13/cobra"
)

var benchmarkModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Run benchmarks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return benchmark.RunBenchmarkModels(GetConfig())
	},
}

func init() {
	benchmarkCmd.AddCommand(benchmarkModelsCmd)
}
