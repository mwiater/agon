// internal/cli/list.go
package agon

import (
	"github.com/spf13/cobra"
)

// listCmd represents the 'list' command group and acts as a namespace
// for subcommands that list information (for example, commands or models).
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Group commands for listing resources",
	Long:  `The 'list' command groups related subcommands that list resources or information. It performs no action on its own.`,
}

func init() {
	rootCmd.AddCommand(listCmd)
}
