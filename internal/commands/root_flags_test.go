package agon

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mwiater/agon/internal/logging"
	"github.com/spf13/viper"
)

func resetFlag(cmdFlag string) {
	flag := rootCmd.PersistentFlags().Lookup(cmdFlag)
	if flag == nil {
		return
	}
	_ = flag.Value.Set(flag.DefValue)
	flag.Changed = false
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestPersistentPreRunEUsesFlagValues(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "agon.log")
	configPath := writeTempConfig(t, "{}")

	prevCfgFile := cfgFile
	cfgFile = configPath
	viper.SetConfigFile(configPath)
	t.Cleanup(func() {
		cfgFile = prevCfgFile
		viper.SetConfigFile(prevCfgFile)
	})
	t.Cleanup(func() { _ = logging.Close() })

	for _, name := range []string{"debug", "multimodelMode", "pipelineMode", "jsonMode", "mcpMode", "mcpBinary", "mcpInitTimeout", "export", "exportMarkdown"} {
		resetFlag(name)
	}
	_ = rootCmd.PersistentFlags().Set("debug", "true")
	_ = rootCmd.PersistentFlags().Set("jsonMode", "true")
	_ = rootCmd.PersistentFlags().Set("mcpMode", "true")
	_ = rootCmd.PersistentFlags().Set("mcpBinary", "custom-mcp")
	_ = rootCmd.PersistentFlags().Set("mcpInitTimeout", "12")
	_ = rootCmd.PersistentFlags().Set("export", "out.json")
	_ = rootCmd.PersistentFlags().Set("exportMarkdown", "out.md")
	_ = rootCmd.PersistentFlags().Set("logFile", logPath)

	if err := rootCmd.PersistentPreRunE(rootCmd, []string{}); err != nil {
		t.Fatalf("PersistentPreRunE error: %v", err)
	}

	if currentConfig == nil || currentConfig.ConfigPath != configPath {
		t.Fatalf("expected config loaded with path %s", configPath)
	}
	if !currentConfig.Debug || !currentConfig.JSONMode || !currentConfig.MCPMode {
		t.Fatalf("expected flag values to flow into config: %+v", currentConfig)
	}
	if currentConfig.MCPBinary != "custom-mcp" {
		t.Fatalf("expected mcpBinary set, got %s", currentConfig.MCPBinary)
	}
	if currentConfig.MCPInitTimeout != 12 {
		t.Fatalf("expected mcpInitTimeout set, got %d", currentConfig.MCPInitTimeout)
	}
}

func TestPersistentPreRunEInvalidModes(t *testing.T) {
	configPath := writeTempConfig(t, "{}")

	prevCfgFile := cfgFile
	cfgFile = configPath
	viper.SetConfigFile(configPath)
	t.Cleanup(func() {
		cfgFile = prevCfgFile
		viper.SetConfigFile(prevCfgFile)
	})
	t.Cleanup(func() { _ = logging.Close() })

	for _, name := range []string{"multimodelMode", "pipelineMode"} {
		resetFlag(name)
	}
	_ = rootCmd.PersistentFlags().Set("multimodelMode", "true")
	_ = rootCmd.PersistentFlags().Set("pipelineMode", "true")

	if err := rootCmd.PersistentPreRunE(rootCmd, []string{}); err == nil {
		t.Fatalf("expected error for invalid mode combination")
	}
}

func TestShowConfigCommandOutput(t *testing.T) {
	configPath := writeTempConfig(t, "{}")

	prevCfgFile := cfgFile
	cfgFile = configPath
	viper.SetConfigFile(configPath)
	t.Cleanup(func() {
		cfgFile = prevCfgFile
		viper.SetConfigFile(prevCfgFile)
	})
	t.Cleanup(func() { _ = logging.Close() })

	for _, name := range []string{"debug", "multimodelMode", "pipelineMode", "jsonMode", "mcpMode"} {
		resetFlag(name)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--debug", "show", "config"})
	t.Cleanup(func() { rootCmd.SetArgs([]string{}) })
	_, err := rootCmd.ExecuteC()
	if err != nil {
		t.Fatalf("ExecuteC error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Config file: "+configPath) {
		t.Fatalf("expected config file path in output, got %s", out)
	}
	if !strings.Contains(out, "Debug:           true") {
		t.Fatalf("expected debug in output, got %s", out)
	}
}
