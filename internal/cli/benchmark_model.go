package agon

import "github.com/spf13/cobra"

var benchmarkModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Run a single benchmark against a benchmark server endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBenchmarkModel(cmd)
	},
}

func init() {
	benchmarkCmd.AddCommand(benchmarkModelCmd)

	benchmarkModelCmd.Flags().StringP("model", "m", "", "model name to benchmark")
	benchmarkModelCmd.Flags().StringP("gpu", "g", "", "GPU name for output filename")
	benchmarkModelCmd.Flags().StringP("benchmark-endpoint", "b", "", "benchmark server endpoint URL")
	_ = benchmarkModelCmd.MarkFlagRequired("model")
	_ = benchmarkModelCmd.MarkFlagRequired("gpu")
	_ = benchmarkModelCmd.MarkFlagRequired("benchmark-endpoint")
}
