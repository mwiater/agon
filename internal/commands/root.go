// internal/commands/root.go
package agon

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile       string
	currentConfig *appconfig.Config
	appVersion    = "dev"
	appCommit     = "none"
	appDate       = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "agon",
	Short: "agon — terminal-first companion for multi-host llama.cpp workflows",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureConfigLoaded(); err != nil {
			return err
		}

		for _, name := range []string{"debug", "multimodelMode", "pipelineMode", "jsonMode", "mcpMode"} {
			if !cmd.Flags().Changed(name) {
				val := viper.GetBool(name)
				_ = cmd.Flags().Set(name, strconv.FormatBool(val))
			}
		}
		for _, name := range []string{"export", "exportMarkdown", "mcpBinary"} {
			if !cmd.Flags().Changed(name) {
				_ = cmd.Flags().Set(name, viper.GetString(name))
			}
		}
		if !cmd.Flags().Changed("mcpInitTimeout") {
			_ = cmd.Flags().Set("mcpInitTimeout", strconv.Itoa(viper.GetInt("mcpInitTimeout")))
		}

		var cfg appconfig.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("unmarshal config: %w", err)
		}
		cfg.ConfigPath = cfgFile
		if cfg.MultimodelMode && cfg.PipelineMode {
			return fmt.Errorf("invalid configuration: only one of multimodelMode or pipelineMode can be enabled")
		}
		currentConfig = &cfg

		if err := logging.Init(currentConfig.LogFilePath()); err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", appVersion, appCommit, appDate)

	defer logging.Close()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config/config.json", "config file (e.g., config/config.json)")

	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	rootCmd.PersistentFlags().Bool("multimodelMode", false, "enable multi-model mode")
	rootCmd.PersistentFlags().Bool("pipelineMode", false, "enable pipeline mode")
	rootCmd.PersistentFlags().Bool("jsonMode", false, "enable JSON output mode")
	rootCmd.PersistentFlags().Bool("mcpMode", false, "proxy LLM traffic through the MCP server")
	rootCmd.PersistentFlags().String("mcpBinary", "", "path to the MCP server binary (defaults per OS)")
	rootCmd.PersistentFlags().Int("mcpInitTimeout", 0, "seconds to wait for MCP startup (0 = default)")
	rootCmd.PersistentFlags().String("export", "", "write pipeline runs to this JSON file")
	rootCmd.PersistentFlags().String("exportMarkdown", "", "write pipeline runs to this Markdown file")
	rootCmd.PersistentFlags().String("logFile", "", "path to the log file")

	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("multimodelMode", rootCmd.PersistentFlags().Lookup("multimodelMode"))
	_ = viper.BindPFlag("pipelineMode", rootCmd.PersistentFlags().Lookup("pipelineMode"))
	_ = viper.BindPFlag("jsonMode", rootCmd.PersistentFlags().Lookup("jsonMode"))
	_ = viper.BindPFlag("mcpMode", rootCmd.PersistentFlags().Lookup("mcpMode"))
	_ = viper.BindPFlag("mcpBinary", rootCmd.PersistentFlags().Lookup("mcpBinary"))
	_ = viper.BindPFlag("mcpInitTimeout", rootCmd.PersistentFlags().Lookup("mcpInitTimeout"))
	_ = viper.BindPFlag("export", rootCmd.PersistentFlags().Lookup("export"))
	_ = viper.BindPFlag("exportMarkdown", rootCmd.PersistentFlags().Lookup("exportMarkdown"))
	_ = viper.BindPFlag("logFile", rootCmd.PersistentFlags().Lookup("logFile"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}
}

// ensureConfigLoaded reads the config and sets safe defaults.
func ensureConfigLoaded() error {
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		return fmt.Errorf("failed to load config: %w", err)
	}
	return nil
}

// GetConfig returns the loaded application configuration for other packages.
func GetConfig() *appconfig.Config {
	return currentConfig
}

// DebugEnabled returns true if debug mode is enabled.
func DebugEnabled() bool { return viper.GetBool("debug") }

// MultiModelEnabled returns true if multi-model mode is enabled.
func MultiModelEnabled() bool { return viper.GetBool("multimodelMode") }

// PipelineModeEnabled returns true if pipeline mode is enabled.
func PipelineModeEnabled() bool { return viper.GetBool("pipelineMode") }

// JSONModeEnabled returns true if JSON mode is enabled.
func JSONModeEnabled() bool { return viper.GetBool("jsonMode") }

// SetVersionInfo allows the main package to inject build-time variables.
func SetVersionInfo(version, commit, date string) {
	appVersion = version
	appCommit = commit
	appDate = date
}
