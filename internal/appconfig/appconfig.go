// internal/appconfig/appconfig.go
// Package appconfig manages loading and interpreting application configuration.
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	// DefaultConfigPath is the default path to the application's configuration file.
	DefaultConfigPath = "config/config.json"
	// legacyConfigPath is the path to the configuration file used in previous
	// versions of the application.
	legacyConfigPath = "config.json"
	// defaultRequestTimeout is the default timeout for HTTP requests made by the
	// application.
	defaultRequestTimeout = 120 * time.Second
)

// Config represents the top-level configuration for the application. It includes a
// list of hosts, as well as global settings for debugging, multimodel mode,
// JSON output, and request timeouts.
type Config struct {
	Hosts          []Host `json:"hosts"`
	Debug          bool   `json:"debug"`
	MultimodelMode bool   `json:"multimodelMode"`
	JSONMode       bool   `json:"jsonMode"`
	TimeoutSeconds int    `json:"timeout,omitempty"`
}

// Host represents a single host that can serve language models. It contains the
// host's name, URL, type, a list of available models, a system prompt, and
// model-specific parameters.
type Host struct {
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Type         string     `json:"type"`
	Models       []string   `json:"models"`
	SystemPrompt string     `json:"systemprompt"`
	Parameters   Parameters `json:"parameters"`
}

// Parameters defines the set of parameters that can be used to control the
// behavior of a language model. These parameters can be adjusted to fine-tune
// the model's output, such as its creativity, verbosity, and adherence to the
// prompt.
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

// RequestTimeout returns the timeout duration for HTTP requests. If the timeout is
// not specified in the configuration, it returns the default timeout.
func (c Config) RequestTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return defaultRequestTimeout
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// Load reads the application configuration from the specified path. If the path
// is empty, it uses the default path. It also supports a legacy configuration
// path for backward compatibility. The function returns a Config struct and an
// error if the configuration cannot be loaded.
func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultConfigPath
	}

	config, err := loadFromPath(path)
	if err == nil {
		if len(config.Hosts) == 0 {
			return Config{}, errors.New("config must contain at least one host")
		}
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

// loadFromPath is a helper function that loads the configuration from a specific
// file path. It opens the file, decodes the JSON content into a Config struct,
// and returns the configuration. If the file cannot be opened or the JSON is
// invalid, it returns an error.
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
	return config, nil
}
