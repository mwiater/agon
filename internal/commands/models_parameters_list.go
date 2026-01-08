// internal/commands/models_parameters_list.go
package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// listModelParametersCmd implements 'list modelParameters', which enumerates
// all models on each configured host and prints their current parameters.
var listModelParametersCmd = &cobra.Command{
	Use:   "modelParameters",
	Short: "List parameters for each model on each node",
	Long:  `The 'modelParameters' subcommand iterates models on each configured node and prints their current parameters (router mode recommended).`,
	Run: func(cmd *cobra.Command, args []string) {
		models.ListModelParameters(GetConfig())
	},
}

func init() {
	listCmd.AddCommand(listModelParametersCmd)
}
