package providerfactory

import (
	"context"
	"fmt"

	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/mcplog"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/mcp"
	"github.com/mwiater/agon/internal/providers/ollama"
)

// NewChatProvider selects the appropriate chat provider implementation based on configuration.
func NewChatProvider(cfg *appconfig.Config) (providers.ChatProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config provided to provider factory")
	}

	if cfg.MCPMode {
		provider, err := mcp.New(context.Background(), cfg)
		if err != nil {
			mcplog.Write(cfg, "MCP provider unavailable: %v", err)
			return nil, err
		}
		mcplog.Write(cfg, "MCP provider ready: using local server")
		return provider, nil
	}

	return ollama.New(cfg), nil
}
