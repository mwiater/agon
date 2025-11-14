// internal/cli/pull.go
package agon

import (
	"github.com/spf13/cobra"
)

// pullCmd represents the 'pull' command group for pulling resources.
var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Group commands for pulling resources",
	Long:  `The 'pull' command groups subcommands that pull resources or information related to agon.`,
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
