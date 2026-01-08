// internal/commands/analyze.go
package agon

import (
	"github.com/spf13/cobra"
)

// analyzeCmd hosts commands that inspect benchmark output and build reports.
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze benchmark outputs",
	Long: `Tools for post-processing benchmark runs. Use these commands to turn raw
benchmark JSON into richer analysis artifacts such as interactive HTML reports.`,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}
