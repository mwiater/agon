// cli/cli_pipeline.go
// Package cli contains the interactive terminal interfaces for Agon, including
// the pipeline mode UI defined in this file.
package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pipelineModel struct {
	message string
}

func initialPipelineModel(cfg *Config) pipelineModel {
	msg := "Pipeline mode is not yet implemented."
	if cfg != nil {
		msg = fmt.Sprintf("Pipeline mode is not yet implemented.\n\nHosts configured: %d", len(cfg.Hosts))
	}
	return pipelineModel{message: msg}
}

func (m pipelineModel) Init() tea.Cmd { return nil }

func (m pipelineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pipelineModel) View() string {
	style := lipgloss.NewStyle().Margin(1, 2)
	return style.Render(m.message + "\n\nPress q to exit.")
}

func StartPipelineGUI(cfg *Config) error {
	m := initialPipelineModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
