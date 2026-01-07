// internal/cli/accuracy.go
package agon

import (
	"log"

	"github.com/mwiater/agon/internal/accuracy"
	"github.com/spf13/cobra"
)

// accuracyCmd represents the accuracy command.
var accuracyCmd = &cobra.Command{
	Use:   "accuracy",
	Short: "Run accuracy checks for models defined in the config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Println("accuracy command called")
		cfg := GetConfig()
		if cfg == nil {
			log.Println("config is nil")
			return nil
		}
		log.Printf("accuracy mode: %v", cfg.AccuracyMode)
		return accuracy.RunAccuracy(GetConfig())
	},
}

func init() {
	rootCmd.AddCommand(accuracyCmd)
}
