// internal/commands/models_sync_configs.go
package agon

import (
	"github.com/spf13/cobra"

	"github.com/mwiater/agon/internal/models"
)

// syncConfigsCmd
var syncConfigsCmd = &cobra.Command{
	Use:   "configs",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		models.SyncConfigs(GetConfig())
	},
}

func init() {
	syncCmd.AddCommand(syncConfigsCmd)
}
