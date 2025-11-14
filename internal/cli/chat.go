// internal/cli/chat.go
package agon

import (
	"context"
	"log"

	"github.com/mwiater/agon/cli"
	"github.com/mwiater/agon/internal/metrics"
	"github.com/spf13/cobra"
)

var (
	// startGUI is a function alias to cli.StartGUI for starting the main chat interface.
	startGUI = cli.StartGUI
	// startPipelineGUI is a function alias to cli.StartPipelineGUI for starting the pipeline chat interface.
	startPipelineGUI = cli.StartPipelineGUI
)

// chatCmd represents the 'chat' command, which starts an interactive chat session.
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start a chat session",
	Long:  `The 'chat' command starts an interactive chat session with a large language model.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())

		cfg := GetConfig()
		metrics.GetInstance().SetMetricsEnabled(true) // Enable metrics for chat mode
		if cfg == nil {
			startGUI(ctx, cfg, cancel)
			return
		}

		if cfg.PipelineMode {
			if err := startPipelineGUI(ctx, cfg, cancel); err != nil {
				log.Fatalf("Error running pipeline program: %v", err)
			}
			return
		}

		startGUI(ctx, cfg, cancel)
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
