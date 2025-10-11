// internal/cli/list_models.go
package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// listModelsCmd implements 'list models', which enumerates all models on
// each configured host and indicates which models are currently loaded.
var listModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List all models on each node",
	Long:  `The 'models' subcommand lists all models on each node specified in the configuration file (default: config/config.json).`,
	Run: func(cmd *cobra.Command, args []string) {
		models.ListModels(getConfig())
	},
}

func init() {
	listCmd.AddCommand(listModelsCmd)
}
