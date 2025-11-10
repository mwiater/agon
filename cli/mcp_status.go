// cli/mcp_status.go
package cli

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mwiater/agon/internal/providers"
	mcpprovider "github.com/mwiater/agon/internal/providers/mcp"
)

// mcpStatus represents the current status of the Multi-Chat Provider (MCP).
type mcpStatus string

const (
	// mcpStatusOff indicates that MCP mode is disabled.
	mcpStatusOff mcpStatus = "off"
	// mcpStatusActive indicates that MCP mode is active and the MCP provider is in use.
	mcpStatusActive mcpStatus = "active"
	// mcpStatusFallback indicates that MCP mode is enabled but the fallback provider is being used.
	mcpStatusFallback mcpStatus = "fallback"
)

// deriveMCPStatus determines the current MCP status based on the configuration and active provider.
func deriveMCPStatus(cfg *Config, provider providers.ChatProvider) mcpStatus {
	if cfg == nil || !cfg.MCPMode {
		return mcpStatusOff
	}
	if provider == nil {
		return mcpStatusFallback
	}
	if _, ok := provider.(*mcpprovider.Provider); ok {
		return mcpStatusActive
	}
	return mcpStatusFallback
}

// formatMCPIndicator returns a human-readable string for the given MCP status.
func formatMCPIndicator(status mcpStatus) string {
	switch status {
	case mcpStatusActive:
		return "MCP Mode: active"
	case mcpStatusFallback:
		return "MCP Mode: fallback"
	default:
		return "MCP Mode: off"
	}
}

// renderMCPBadge returns a Lipgloss-styled badge string for the MCP status.
func renderMCPBadge(status mcpStatus) string {
	badgeStyle := lipgloss.NewStyle().Background(lipgloss.Color("229")).Foreground(lipgloss.Color("0")).Padding(0, 1).MarginLeft(1)
	return badgeStyle.Render(formatMCPIndicator(status))
}