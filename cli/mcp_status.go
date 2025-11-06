package cli

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mwiater/agon/internal/providers"
	mcpprovider "github.com/mwiater/agon/internal/providers/mcp"
)

type mcpStatus string

const (
	mcpStatusOff      mcpStatus = "off"
	mcpStatusActive   mcpStatus = "active"
	mcpStatusFallback mcpStatus = "fallback"
)

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

func renderMCPBadge(status mcpStatus) string {
	badgeStyle := lipgloss.NewStyle().Background(lipgloss.Color("229")).Foreground(lipgloss.Color("0")).Padding(0, 1).MarginLeft(1)
	return badgeStyle.Render(formatMCPIndicator(status))
}
