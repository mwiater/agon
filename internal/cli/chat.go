// internal/cli/chat.go
package agon

import (
	"github.com/mwiater/agon/cli"
	"github.com/spf13/cobra"
)

var startGUI = cli.StartGUI

// chatCmd represents the 'chat' command.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start a chat session",
	Long:  `The 'chat' command starts an interactive chat session with a large language model.`,
	Run: func(cmd *cobra.Command, args []string) {
		startGUI(getConfig())
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
