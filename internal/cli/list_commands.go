// internal/cli/list_commands.go
package agon

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// commandsCmd implements 'list commands', which prints the available
// commands and subcommands in a hierarchical, indented, two-column format.
var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "List all commands and subcommands in two columns",
	Long:  `The 'commands' subcommand lists all commands and subcommands in a hierarchical, indented format, with the command path in the first column and its short description in the second column.`,
	Run: func(cmd *cobra.Command, args []string) {
		listAllCommands(rootCmd)
	},
}

func init() {
	listCmd.AddCommand(commandsCmd)
}

// listAllCommands recursively traverses the command tree starting from rootCmd
// and prints each command path and short description in a padded, two-column layout.
func listAllCommands(rootCmd *cobra.Command) {
	commandData := collectCommandData(rootCmd, "", "")

	maxPathLength := 0
	for _, data := range commandData {
		if len(data.path) > maxPathLength {
			maxPathLength = len(data.path)
		}
	}

	fmt.Println("Commands and Subcommands:")
	for _, data := range commandData {
		if strings.Contains(data.path, "completion") {
			continue
		}
		fmt.Printf("  %s%s%s\n", data.path, strings.Repeat(" ", maxPathLength-len(data.path)+2), data.description)
	}
}

// commandInfo holds the path and description of a command for display.
type commandInfo struct {
	path        string
	description string
}

// collectCommandData collects command metadata for display, walking the
// command tree and returning a flattened slice of path/description pairs.
func collectCommandData(cmd *cobra.Command, currentPath string, indent string) []commandInfo {
	var allData []commandInfo

	fullPath := currentPath + cmd.Name()
	if currentPath != "" {
		fullPath = currentPath + " " + cmd.Name()
	}

	data := commandInfo{
		path:        indent + fullPath,
		description: cmd.Short,
	}

	allData = append(allData, data)

	for _, subCmd := range cmd.Commands() {
		allData = append(allData, collectCommandData(subCmd, fullPath, indent+"  ")...)
	}

	return allData
}
