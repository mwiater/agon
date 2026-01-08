// internal/commands/chat.go
package agon

import (
	"github.com/mwiater/agon/internal/chat"
	"github.com/mwiater/agon/internal/tui"
	"github.com/spf13/cobra"
)

var (
	// startGUI is a function alias to tui.StartGUI for starting the main chat interface.
	startGUI = tui.StartGUI
	// startPipelineGUI is a function alias to tui.StartPipelineGUI for starting the pipeline chat interface.
	startPipelineGUI = tui.StartPipelineGUI
)

// chatCmd represents the 'chat' command, which starts an interactive chat session.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start a chat session",
	Long:  `The 'chat' command starts an interactive chat session with a large language model.`,
	Run: func(cmd *cobra.Command, args []string) {
		chat.Run(GetConfig(), startGUI, startPipelineGUI)
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
