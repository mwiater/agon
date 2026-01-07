// internal/cli/run.go
package agon

import "github.com/spf13/cobra"

// runCmd represents the 'run' command group for running workflows.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Group commands for running workflows",
	Long:  `The 'run' command groups subcommands that run higher-level workflows.`,
}

func init() {
	rootCmd.AddCommand(runCmd)
}
