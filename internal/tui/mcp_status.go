// internal/tui/mcp_status.go
package tui

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
)

// deriveMCPStatus determines the current MCP status based on the configuration and active provider.
func deriveMCPStatus(cfg *Config, provider providers.ChatProvider) mcpStatus {
	if cfg == nil || !cfg.MCPMode {
		return mcpStatusOff
	}
	unwrapped := unwrapProvider(provider)
	if _, ok := unwrapped.(*mcpprovider.Provider); ok {
		return mcpStatusActive
	}
	return mcpStatusOff
}

func unwrapProvider(provider providers.ChatProvider) providers.ChatProvider {
	for {
		if provider == nil {
			return nil
		}
		if wrapper, ok := provider.(interface{ Wrapped() providers.ChatProvider }); ok {
			next := wrapper.Wrapped()
			if next == provider {
				return provider
			}
			provider = next
			continue
		}
		return provider
	}
}

// formatMCPIndicator returns a human-readable string for the given MCP status.
func formatMCPIndicator(status mcpStatus) string {
	switch status {
	case mcpStatusActive:
		return "MCP Mode: active"
	default:
		return "MCP Mode: off"
	}
}

// renderMCPBadge returns a Lipgloss-styled badge string for the MCP status.
func renderMCPBadge(status mcpStatus) string {
	badgeStyle := lipgloss.NewStyle().Background(lipgloss.Color("229")).Foreground(lipgloss.Color("0")).Padding(0, 1).MarginLeft(1)
	return badgeStyle.Render(formatMCPIndicator(status))
}

// renderJSONBadge returns a Lipgloss-styled badge string for JSON mode.
func renderJSONBadge(enabled bool) string {
	label := "JSON Mode: off"
	if enabled {
		label = "JSON Mode: on"
	}
	badgeStyle := lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0")).Padding(0, 1)
	return badgeStyle.Render(label)
}
