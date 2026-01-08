// internal/commands/unload.go
package agon

import (
	"github.com/spf13/cobra"
)

// unloadCmd represents the 'unload' command group for unloading resources.
var unloadCmd = &cobra.Command{
	Use:   "unload",
	Short: "Group commands for unloading resources",
	Long:  `The 'unload' command groups subcommands that unload resources from supported hosts.`,
}

func init() {
	rootCmd.AddCommand(unloadCmd)
}
