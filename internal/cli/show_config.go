// internal/cli/show_config.go
package agon

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// syncModelsCmd implements 'sync models', which deletes models not in the
// configuration and then pulls any missing models across supported hosts.
var showConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show config settings",
	Long:  `Show config settings ensuring that the JSON configs are loaded properly and overriden by flags accordingly.`,
	Run: func(cmd *cobra.Command, args []string) {
		file := viper.ConfigFileUsed()
		if file == "" {
			fmt.Println("No config file loaded (using defaults).")
		} else {
			fmt.Printf("Config file: %s\n\n", file)
		}

		fmt.Println("Current configuration:")
		fmt.Printf("  Debug:           %v\n", viper.GetBool("debug"))
		fmt.Printf("  Multimodel Mode: %v\n", viper.GetBool("multimodelMode"))
		fmt.Printf("  JSON Mode:       %v\n", viper.GetBool("jsonMode"))
	},
}

func init() {
	showCmd.AddCommand(showConfigCmd)
}
