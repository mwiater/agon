// internal/cli/list_commands.go
package agon

import "github.com/spf13/cobra"

// commandsCmd implements 'list commands', which prints the available
// commands and subcommands in a hierarchical, indented, two-column format.
var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List all commands and subcommands in two columns",
	Long:  `The 'commands' subcommand lists all commands and subcommands in a hierarchical, indented format, with the command path in the first column and its short description in the second column.`,
	Run: func(cmd *cobra.Command, args []string) {
		runListCommands(rootCmd)
	},
}

func init() {
	listCmd.AddCommand(commandsCmd)
}
