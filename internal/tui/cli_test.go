// internal/tui/cli_test.go
package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestUpdate tests the Update function of the Bubble Tea model. It verifies
// that the model correctly handles various messages, such as key presses (e.g.,
// quit, navigation) and window size changes. The test ensures that the model's
// state transitions are accurate and that the appropriate commands are generated
// in response to user input and system events.
func TestUpdate(t *testing.T) {
	cfg := &Config{
		Hosts: []Host{
			{
				Name:   "Test Host",
				URL:    "http://localhost:11434",
				Models: []string{"model1", "model2"},
			},
		},
	}
	provider := newTestProvider()
	provider.loadedModels["Test Host"] = []string{"model1"}
	m := initialModel(context.Background(), cfg, provider)

	if m.state != viewHostSelector {
		t.Errorf("Expected initial state to be viewHostSelector, got %v", m.state)
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("Expected a quit command, but got nil")
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Expected a quit command, but got nil")
	}

	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 100})
	m = newModel.(*model)
	if m.width != 100 || m.height != 100 {
		t.Errorf("Expected width and height to be 100, got %d and %d", m.width, m.height)
	}

	m = initialModel(context.Background(), cfg, provider)

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*model)

	if m.state != viewHostSelector {
		t.Errorf("Expected state to be viewHostSelector, got %v", m.state)
	}
}

// TestView tests the View function of the Bubble Tea model. It checks that the
// correct UI is rendered for different states of the application, such as the
// initial loading screen, error messages, and the host selection view. The test
// ensures that the rendered output contains the expected text and that the UI
// accurately reflects the model's current state.
func TestView(t *testing.T) {
	cfg := &Config{
		Hosts: []Host{
			{
				Name:   "Test Host",
				URL:    "http://localhost:11434",
				Models: []string{"model1", "model2"},
			},
		},
	}
	provider := newTestProvider()
	provider.loadedModels["Test Host"] = []string{"model1"}
	m := initialModel(context.Background(), cfg, provider)

	m.width = 0
	view := m.View()
	if view != "Initializing..." {
		t.Errorf("Expected view to be 'Initializing...', got '%s'", view)
	}

	m.width = 100
	m.err = modelsLoadErr{error: errors.New("test error")}
	view = m.View()
	if !strings.Contains(view, "Error") {
		t.Errorf("Expected view to contain 'Error', got '%s'", view)
	}
	m.err = nil

	view = m.View()
	if !strings.Contains(view, "Select a Host") {
		t.Errorf("Expected view to contain 'Select a Host', got '%s'", view)
	}
}
