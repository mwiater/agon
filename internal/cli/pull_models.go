// internal/cli/pull_models.go
package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// pullModelsCmd implements 'pull models', which pulls all configured models
// to each supported host defined in the configuration file.
var pullModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Pull all models from the configuration file",
	Long:  `The 'models' subcommand pulls all models from the configuration file (default: config/config.json).`,
	Run: func(cmd *cobra.Command, args []string) {
		models.PullModels(GetConfig())
	},
}

func init() {
	pullCmd.AddCommand(pullModelsCmd)
}
