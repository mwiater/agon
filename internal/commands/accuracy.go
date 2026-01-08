// internal/commands/accuracy.go
package agon

import (
	"github.com/mwiater/agon/internal/accuracy"
	"github.com/spf13/cobra"
)

// accuracyCmd represents the accuracy command.
var accuracyCmd = &cobra.Command{
	Use:   "accuracy",
	Short: "Run accuracy checks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		return accuracy.RunAccuracyCommand(GetConfig())
	},
}

func init() {
	rootCmd.AddCommand(accuracyCmd)
}
