package agon

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// showConfigCmd implements the 'show config' command, which displays the current configuration settings.
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

		cfg := GetConfig()
		fmt.Println("Current configuration:")
		if cfg == nil {
			fmt.Printf("  Debug:           %v\n", viper.GetBool("debug"))
			fmt.Printf("  Multimodel Mode: %v\n", viper.GetBool("multimodelMode"))
			fmt.Printf("  Pipeline Mode:   %v\n", viper.GetBool("pipelineMode"))
			fmt.Printf("  JSON Mode:       %v\n", viper.GetBool("jsonMode"))
			fmt.Printf("  MCP Mode:        %v\n", viper.GetBool("mcpMode"))
			fmt.Printf("  MCP Binary:      %s\n", viper.GetString("mcpBinary"))
			fmt.Printf("  MCP Init Timeout: %d seconds\n", viper.GetInt("mcpInitTimeout"))
			fmt.Printf("  Export JSON:     %s\n", viper.GetString("export"))
			fmt.Printf("  Export Markdown: %s\n", viper.GetString("exportMarkdown"))
			return
		}

		fmt.Printf("  Debug:           %v\n", cfg.Debug)
		fmt.Printf("  Multimodel Mode: %v\n", cfg.MultimodelMode)
		fmt.Printf("  Pipeline Mode:   %v\n", cfg.PipelineMode)
		fmt.Printf("  JSON Mode:       %v\n", cfg.JSONMode)
		fmt.Printf("  MCP Mode:        %v\n", cfg.MCPMode)
		fmt.Printf("  MCP Binary:      %s\n", cfg.MCPBinaryPath())
		fmt.Printf("  MCP Init Timeout: %s\n", cfg.MCPInitTimeoutDuration())
		fmt.Printf("  Export JSON:     %s\n", cfg.ExportPath)
		fmt.Printf("  Export Markdown: %s\n", cfg.ExportMarkdownPath)
	},
}

func init() {
	showCmd.AddCommand(showConfigCmd)
}