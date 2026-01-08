// internal/commands/delete.go
package agon

import (
	"github.com/spf13/cobra"
)

// deleteCmd represents the 'delete' command group for deleting resources.
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Group commands for deleting resources",
	Long:  `The 'delete' command groups subcommands that delete resources or information related to agon.`,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
