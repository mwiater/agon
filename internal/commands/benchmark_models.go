package agon

import (
	"strings"

	"github.com/mwiater/agon/internal/benchmark"
	"github.com/spf13/cobra"
)

var benchmarkModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Run benchmarks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(benchmarkModelsGPU) != "" || strings.TrimSpace(benchmarkModelsServer) != "" {
			return benchmark.RunBenchmarkModelsFromMetadata(benchmarkModelsGPU, benchmarkModelsServer)
		}
		return benchmark.RunBenchmarkModels(GetConfig())
	},
}

var benchmarkModelsGPU string
var benchmarkModelsServer string

func init() {
	benchmarkCmd.AddCommand(benchmarkModelsCmd)
	benchmarkModelsCmd.Flags().StringVar(&benchmarkModelsGPU, "gpu", "", "GPU identifier used to filter metadata and filenames")
	benchmarkModelsCmd.Flags().StringVar(&benchmarkModelsServer, "benchmark-server", "", "Benchmark server endpoint URL")
}
