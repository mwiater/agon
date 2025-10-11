// internal/cli/root.go
package agon

import (
	"fmt"
	"os"
	"sync"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// rootCmd is the base Cobra command for the agon application.
// All subcommands are attached to this root to form the complete CLI.
var rootCmd = &cobra.Command{
	Use:   "agon",
	Short: "agon",
	Long:  `agon`,
}

var (
	cfgFile string

	cfgOnce       sync.Once
	cfgLoadErr    error
	currentConfig *appconfig.Config
)

// Execute runs the root Cobra command and all registered subcommands.
// It prints any returned error and exits the process with a non-zero
// status code on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// init initializes the root command by setting up persistent flags and the
// PersistentPreRunE function. This ensures that the application configuration
// is loaded before any subcommand is executed.
func init() {
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return ensureConfigLoaded()
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", appconfig.DefaultConfigPath, "config file (e.g., config/config.Authors.json)")
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
}

// ensureConfigLoaded loads the application configuration using a sync.Once to
// ensure it is loaded only once. It reads the configuration path from Viper,
// loads the configuration file, and stores it in a global variable. Any errors
// that occur during loading are stored and returned on subsequent calls.
func ensureConfigLoaded() error {
	cfgOnce.Do(func() {
		configPath := viper.GetString("config")
		cfg, err := appconfig.Load(configPath)
		if err != nil {
			cfgLoadErr = err
			return
		}
		currentConfig = &cfg
	})
	return cfgLoadErr
}

// getConfig returns the loaded application configuration. It is a helper
// function to access the configuration from other parts of the CLI.
func getConfig() *appconfig.Config {
	return currentConfig
}