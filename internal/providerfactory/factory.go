// internal/providerfactory/factory.go
package providerfactory

import (
	"context"
	"fmt"
	"strings"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/metrics"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/llamacpp"
	"github.com/mwiater/agon/internal/providers/mcp"
	"github.com/mwiater/agon/internal/providers/multiplex"
	"github.com/mwiater/agon/internal/providers/ollama"
)

// NewChatProvider selects and configures the appropriate chat provider based on the
// application configuration. It will choose between the MCP and Ollama providers
// and wrap the selected provider with metrics collection if enabled.
func NewChatProvider(cfg *appconfig.Config) (providers.ChatProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config provided to provider factory")
	}

	var provider providers.ChatProvider
	var err error

	if cfg.MCPMode {
		provider, err = mcp.New(context.Background(), cfg)
		if err != nil {
			logging.LogEvent("MCP provider unavailable: %v", err)
			return nil, err
		}
		logging.LogEvent("MCP provider ready: using local server")
	} else {
		hostTypes, err := collectHostTypes(cfg)
		if err != nil {
			return nil, err
		}
		switch len(hostTypes) {
		case 0:
			return nil, fmt.Errorf("no hosts configured")
		case 1:
			if hostTypes["ollama"] {
				provider = ollama.New(cfg)
			} else if hostTypes["llama.cpp"] {
				provider = llamacpp.New(cfg)
			} else {
				return nil, fmt.Errorf("unsupported host type in config")
			}
		default:
			provider = multiplex.New(map[string]providers.ChatProvider{
				"ollama":    ollama.New(cfg),
				"llama.cpp": llamacpp.New(cfg),
			})
		}
	}

	if cfg.Metrics {
		aggregator := metrics.GetInstance()
		provider = metrics.NewProvider(provider, aggregator)
	}

	return provider, nil
}

func collectHostTypes(cfg *appconfig.Config) (map[string]bool, error) {
	types := make(map[string]bool)
	for _, host := range cfg.Hosts {
		normalized := strings.ToLower(strings.TrimSpace(host.Type))
		switch normalized {
		case "", "ollama":
			types["ollama"] = true
		case "llama.cpp", "llamacpp":
			types["llama.cpp"] = true
		default:
			return nil, fmt.Errorf("unsupported host type %q", host.Type)
		}
	}
	return types, nil
}
