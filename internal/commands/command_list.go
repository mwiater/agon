package agon

import (
	"fmt"
	"io"
	"strings"
)

// CommandInfo holds the path and description of a command for display.
type CommandInfo struct {
	Path        string
	Description string
}

// ListCommands prints the command tree in a two-column layout.
func ListCommands(out io.Writer, commands []CommandInfo) {
	maxPathLength := 0
	for _, data := range commands {
		if len(data.Path) > maxPathLength {
			maxPathLength = len(data.Path)
		}
	}

	fmt.Fprintln(out, "Commands and Subcommands:")
	for _, data := range commands {
		fmt.Fprintf(out, "  %s%s%s\n", data.Path, strings.Repeat(" ", maxPathLength-len(data.Path)+2), data.Description)
	}
}
