package agon

import (
	"github.com/mwiater/agon/internal/appconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// showConfigCmd implements the 'show config' command, which displays the current configuration settings.
var showConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show config settings",
	Long:  `Show config settings ensuring that the JSON configs are loaded properly and overriden by flags accordingly.`,
	Run: func(cmd *cobra.Command, args []string) {
		fallback := appconfig.Config{
			Debug:              viper.GetBool("debug"),
			MultimodelMode:     viper.GetBool("multimodelMode"),
			PipelineMode:       viper.GetBool("pipelineMode"),
			JSONMode:           viper.GetBool("jsonMode"),
			MCPMode:            viper.GetBool("mcpMode"),
			MCPBinary:          viper.GetString("mcpBinary"),
			MCPInitTimeout:     viper.GetInt("mcpInitTimeout"),
			ExportPath:         viper.GetString("export"),
			ExportMarkdownPath: viper.GetString("exportMarkdown"),
		}
		appconfig.ShowConfig(cmd.OutOrStdout(), viper.ConfigFileUsed(), GetConfig(), fallback)
	},
}

func init() {
	showCmd.AddCommand(showConfigCmd)
}
