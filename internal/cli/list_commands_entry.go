package agon

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// runListCommands prints the command tree in a two-column layout.
func runListCommands(rootCmd *cobra.Command) {
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
