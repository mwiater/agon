// internal/commands/list_commands.go
package agon

import (
	"strings"

	"github.com/mwiater/agon/internal/commandlist"
	"github.com/spf13/cobra"
)

// commandsCmd implements 'list commands', which prints the available
// commands and subcommands in a hierarchical, indented, two-column format.
var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List all commands and subcommands in two columns",
	Long:  `The 'commands' subcommand lists all commands and subcommands in a hierarchical, indented format, with the command path in the first column and its short description in the second column.`,
	Run: func(cmd *cobra.Command, args []string) {
		commandData := collectCommandData(rootCmd, "", "")
		filtered := make([]commandlist.CommandInfo, 0, len(commandData))
		for _, data := range commandData {
			if strings.Contains(data.Path, "completion") {
				continue
			}
			filtered = append(filtered, data)
		}
		commandlist.ListCommands(cmd.OutOrStdout(), filtered)
	},
}

func init() {
	listCmd.AddCommand(commandsCmd)
}

// collectCommandData collects command metadata for display, walking the
// command tree and returning a flattened slice of path/description pairs.
func collectCommandData(cmd *cobra.Command, currentPath string, indent string) []commandlist.CommandInfo {
	var allData []commandlist.CommandInfo

	fullPath := currentPath + cmd.Name()
	if currentPath != "" {
		fullPath = currentPath + " " + cmd.Name()
	}

	data := commandlist.CommandInfo{
		Path:        indent + fullPath,
		Description: cmd.Short,
	}

	allData = append(allData, data)

	for _, subCmd := range cmd.Commands() {
		allData = append(allData, collectCommandData(subCmd, fullPath, indent+"  ")...)
	}

	return allData
}
