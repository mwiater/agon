// internal/providerfactory/factory.go
package providerfactory

import (
	"context"
	"fmt"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/metrics"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/mcp"
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
		provider = ollama.New(cfg)
	}

	if cfg.Metrics {
		aggregator := metrics.GetInstance()
		provider = metrics.NewProvider(provider, aggregator)
	}

	return provider, nil
}