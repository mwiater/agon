package agon

import (
	"log"

	"github.com/mwiater/agon/benchmark"
	"github.com/spf13/cobra"
)

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run benchmarks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("benchmark command called")
		cfg := GetConfig()
		if cfg == nil {
			log.Println("config is nil")
			return nil
		}
		log.Printf("benchmark mode: %v", cfg.BenchmarkMode)
		return benchmark.BenchmarkModels(GetConfig())
	},
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)
}
