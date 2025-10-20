// internal/cli/chat.go
package agon

import (
	"log"

	"github.com/mwiater/agon/cli"
	"github.com/spf13/cobra"
)

var (
	startGUI         = cli.StartGUI
	startPipelineGUI = cli.StartPipelineGUI
)

// chatCmd represents the 'chat' command.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start a chat session",
	Long:  `The 'chat' command starts an interactive chat session with a large language model.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := getConfig()
		if cfg == nil {
			startGUI(cfg)
			return
		}

		if cfg.PipelineMode {
			if err := startPipelineGUI(cfg); err != nil {
				log.Fatalf("Error running pipeline program: %v", err)
			}
			return
		}

		startGUI(cfg)
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
