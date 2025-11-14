// internal/appconfig/appconfig.go
// Package appconfig manages loading and interpreting application configuration.
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultConfigPath is the default path to the application's configuration file.
	DefaultConfigPath = "config/config.json"
	// legacyConfigPath is the path to the configuration file used in previous versions.
	legacyConfigPath = "config.json"
	// defaultRequestTimeout is the default timeout for HTTP requests.
	defaultRequestTimeout = 600 * time.Second
	// defaultMCPInitTimeout defines the fallback timeout used while initializing the MCP server.
	defaultMCPInitTimeout = 10 * time.Second
	// defaultMCPRetryCount defines how many times MCP tools are retried when the config omits the value.
	defaultMCPRetryCount = 1
)

// Config represents the top-level application configuration.
type Config struct {
	Hosts              []Host `json:"hosts"`
	Debug              bool   `json:"debug"`
	MultimodelMode     bool   `json:"multimodelMode"`
	PipelineMode       bool   `json:"pipelineMode"`
	JSONMode           bool   `json:"jsonMode"`
	MCPMode            bool   `json:"mcpMode"`
	MCPBinary          string `json:"mcpBinary,omitempty"`
	MCPInitTimeout     int    `json:"mcpInitTimeout,omitempty"`
	MCPRetryCount      int    `json:"mcpRetryCount,omitempty"`
	TimeoutSeconds     int    `json:"timeout,omitempty"`
	ExportPath         string `json:"export,omitempty"`
	ExportMarkdownPath string `json:"exportMarkdown,omitempty"`
	LogFile            string `json:"logFile,omitempty"`
	BenchmarkMode      bool   `json:"benchmarkMode"`
	BenchmarkCount     int    `json:"benchmarkCount"`
	Metrics            bool   `json:"metrics"`
	ConfigPath         string `json:"-"`
}

// Host represents a single host that can serve language models.
type Host struct {
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Type         string     `json:"type"`
	Models       []string   `json:"models"`
	SystemPrompt string     `json:"systemprompt"`
	Parameters   Parameters `json:"parameters"`
}

// Parameters defines the set of parameters that can be used to control a language model's behavior.
type Parameters struct {
	TopK             *int     `json:"top_k,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	MinP             *float64 `json:"min_p,omitempty"`
	TFSZ             *float64 `json:"tfs_z,omitempty"`
	TypicalP         *float64 `json:"typical_p,omitempty"`
	RepeatLastN      *int     `json:"repeat_last_n,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	RepeatPenalty    *float64 `json:"repeat_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
}

// RequestTimeout returns the timeout duration for HTTP requests, falling back to the default if not specified.
func (c Config) RequestTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return defaultRequestTimeout
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// MCPInitTimeoutDuration returns the timeout duration for MCP initialization.
func (c Config) MCPInitTimeoutDuration() time.Duration {
	if c.MCPInitTimeout <= 0 {
		return defaultMCPInitTimeout
	}
	return time.Duration(c.MCPInitTimeout) * time.Second
}

// MCPRetryAttempts returns the configured number of retry attempts for MCP tools.
func (c Config) MCPRetryAttempts() int {
	if c.MCPRetryCount < 0 {
		return 0
	}
	if c.MCPRetryCount == 0 {
		return defaultMCPRetryCount
	}
	return c.MCPRetryCount
}

// LogFilePath returns the path to the application log file, applying a default if not set.
func (c Config) LogFilePath() string {
	if path := c.LogFile; strings.TrimSpace(path) != "" {
		return path
	}
	return "agon.log"
}

// MCPBinaryPath returns the resolved MCP server binary path, choosing a default based on the OS if not provided.
func (c Config) MCPBinaryPath() string {
	if b := strings.TrimSpace(c.MCPBinary); b != "" {
		return b
	}
	goos := runtime.GOOS
	switch goos {
	case "windows":
		return "dist/agon-mcp_windows_amd64_v1/agon-mcp.exe"
	case "linux":
		return "dist/agon-mcp_linux_amd64_v1/agon-mcp"
	default:
		return "dist/agon-mcp"
	}
}

// Load reads the application configuration from the specified path, with fallback to a legacy path.
func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	config, err := loadFromPath(path)
	if err == nil {
		if len(config.Hosts) == 0 {
			return Config{}, errors.New("config must contain at least one host")
		}
		config.ConfigPath = path
		return config, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		if path == DefaultConfigPath {
			config, legacyErr := loadFromPath(legacyConfigPath)
			if legacyErr == nil {
				return config, nil
			}
			if errors.Is(legacyErr, os.ErrNotExist) {
				return Config{}, fmt.Errorf("no configuration file found (searched %q and %q)", DefaultConfigPath, legacyConfigPath)
			}
			return Config{}, fmt.Errorf("could not read config file %q: %w", legacyConfigPath, legacyErr)
		}
		return Config{}, fmt.Errorf("no configuration file found at %q", path)
	}

	return Config{}, fmt.Errorf("could not read config file %q: %w", path, err)
}

// loadFromPath is a helper function that loads the configuration from a specific file path.
func loadFromPath(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return Config{}, err
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = int(defaultRequestTimeout.Seconds())
	}

	return config, nil
}
