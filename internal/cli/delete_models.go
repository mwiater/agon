// internal/cli/delete_models.go
package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// deleteModelsCmd implements 'delete models', which removes models not listed
// in the configuration from each supported host.
var deleteModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Delete all models not in the configuration file",
	Long:  `The 'models' subcommand deletes all models not in the configuration file (default: config/config.json).`,
	Run: func(cmd *cobra.Command, args []string) {
		models.DeleteModels(GetConfig())
	},
}

func init() {
	deleteCmd.AddCommand(deleteModelsCmd)
}