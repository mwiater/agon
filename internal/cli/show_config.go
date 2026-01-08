package agon

import "github.com/spf13/cobra"

// showConfigCmd implements the 'show config' command, which displays the current configuration settings.
var showConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show config settings",
	Long:  `Show config settings ensuring that the JSON configs are loaded properly and overriden by flags accordingly.`,
	Run: func(cmd *cobra.Command, args []string) {
		runShowConfig()
	},
}

func init() {
	showCmd.AddCommand(showConfigCmd)
}
