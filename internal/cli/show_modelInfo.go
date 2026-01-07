// internal/cli/show_modelInfo.go

package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// showModelInfoCmd implements the 'show modelInfo' command, which displays the current configuration model details on each host.
var showModelInfoCmd = &cobra.Command{
	Use:   "modelInfo",
	Short: "Show model detailed information from the configuration file",
	Long:  `Show model detailed information from the configuration file`,
	Run: func(cmd *cobra.Command, args []string) {
		models.ShowModelInfo(GetConfig())
	},
}

func init() {
	showCmd.AddCommand(showModelInfoCmd)
}
