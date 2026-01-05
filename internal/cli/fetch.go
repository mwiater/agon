// internal/cli/fetch.go
package agon

import "github.com/spf13/cobra"

// fetchCmd represents the 'fetch' command group and acts as a namespace
// for subcommands that retrieve information from external services.
var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Group commands for fetching resources",
	Long:  "The 'fetch' command groups related subcommands that fetch resources or information. It performs no action on its own.",
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
