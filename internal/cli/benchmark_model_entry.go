package agon

import (
	"github.com/mwiater/agon/internal/benchmark"
	"github.com/spf13/cobra"
)

func runBenchmarkModel(cmd *cobra.Command) error {
	modelName, _ := cmd.Flags().GetString("model")
	gpuName, _ := cmd.Flags().GetString("gpu")
	endpoint, _ := cmd.Flags().GetString("benchmark-endpoint")
	return benchmark.RunBenchmarkModel(modelName, gpuName, endpoint)
}
