// cmd/agon/root.go
package agon

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile       string
	currentConfig *appconfig.Config
)

var rootCmd = &cobra.Command{
	Use:   "agon",
	Short: "agon — terminal-first companion for multi-host Ollama workflows",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 1) Load config (file or defaults)
		if err := ensureConfigLoaded(); err != nil {
			return err
		}

		// 2) If user did NOT set a flag, copy the config value into the flag so
		//    both pflags and viper reflect the same, final value.
		for _, name := range []string{"debug", "multimodelMode", "pipelineMode", "jsonMode"} {
			if !cmd.Flags().Changed(name) {
				val := viper.GetBool(name)
				_ = cmd.Flags().Set(name, strconv.FormatBool(val))
			}
		}
		for _, name := range []string{"export", "exportMarkdown"} {
			if !cmd.Flags().Changed(name) {
				_ = cmd.Flags().Set(name, viper.GetString(name))
			}
		}

		// 3) Materialize the fully merged configuration into currentConfig
		//    (flags > config > defaults). This gives other packages a stable snapshot.
		var cfg appconfig.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("unmarshal config: %w", err)
		}
		if cfg.MultimodelMode && cfg.PipelineMode {
			return fmt.Errorf("invalid configuration: only one of multimodelMode or pipelineMode can be enabled")
		}
		currentConfig = &cfg

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// --config (defaults to your existing path)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config/config.json", "config file (e.g., config/config.json)")

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().Bool("debug", false, "enable debug logging")
	rootCmd.PersistentFlags().Bool("multimodelMode", false, "enable multi-model mode")
	rootCmd.PersistentFlags().Bool("pipelineMode", false, "enable pipeline mode")
	rootCmd.PersistentFlags().Bool("jsonMode", false, "enable JSON output mode")
	rootCmd.PersistentFlags().String("export", "", "write pipeline runs to this JSON file")
	rootCmd.PersistentFlags().String("exportMarkdown", "", "write pipeline runs to this Markdown file")

	// Bind flags to Viper keys (flags override config)
	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("multimodelMode", rootCmd.PersistentFlags().Lookup("multimodelMode"))
	_ = viper.BindPFlag("pipelineMode", rootCmd.PersistentFlags().Lookup("pipelineMode"))
	_ = viper.BindPFlag("jsonMode", rootCmd.PersistentFlags().Lookup("jsonMode"))
	_ = viper.BindPFlag("export", rootCmd.PersistentFlags().Lookup("export"))
	_ = viper.BindPFlag("exportMarkdown", rootCmd.PersistentFlags().Lookup("exportMarkdown"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}
}

// ensureConfigLoaded reads the config and sets safe defaults.
func ensureConfigLoaded() error {
	viper.SetDefault("debug", false)
	viper.SetDefault("multimodelMode", false)
	viper.SetDefault("pipelineMode", false)
	viper.SetDefault("jsonMode", false)
	viper.SetDefault("export", "")
	viper.SetDefault("exportMarkdown", "")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// No file: fine, we'll use defaults/flags
			return nil
		}
		return fmt.Errorf("failed to load config: %w", err)
	}
	return nil
}

// getConfig returns the loaded application configuration for other packages.
func getConfig() *appconfig.Config {
	return currentConfig
}

// Helper accessors (reflect merged Viper state)
func DebugEnabled() bool        { return viper.GetBool("debug") }
func MultiModelEnabled() bool   { return viper.GetBool("multimodelMode") }
func PipelineModeEnabled() bool { return viper.GetBool("pipelineMode") }
func JSONModeEnabled() bool     { return viper.GetBool("jsonMode") }
