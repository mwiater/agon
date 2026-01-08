// internal/commands/benchmark.go
package agon

import "github.com/spf13/cobra"

// benchmarkCmd groups benchmark-related CLI commands.
var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Group commands for running benchmarks",
}

func init() {
	rootCmd.AddCommand(benchmarkCmd)
}
