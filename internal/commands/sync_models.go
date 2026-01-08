// internal/commands/sync_models.go
package agon

import (
	"github.com/spf13/cobra"

	"github.com/mwiater/agon/internal/models"
)

// syncModelsCmd implements 'sync models', which deletes models not in the
// configuration and then pulls any missing models across supported hosts.
var syncModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Sync all models from the configuration file",
	Long:  `The 'models' subcommand syncs all models from the configuration file (default: config/config.json).`,
	Run: func(cmd *cobra.Command, args []string) {
		models.SyncModels(GetConfig())
	},
}

func init() {
	syncCmd.AddCommand(syncModelsCmd)
}
