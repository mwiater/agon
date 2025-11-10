// internal/cli/show.go
package agon

import (
	"github.com/spf13/cobra"
)

// showCmd represents the 'show' command group for displaying resources.
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Group commands for displaying resources",
	Long:  `The 'show' command groups subcommands that display resources or information related to agon.`,
}

func init() {
	rootCmd.AddCommand(showCmd)
}