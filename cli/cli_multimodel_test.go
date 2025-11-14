// cli/cli_multimodel_test.go
package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestInitialMultimodelModel verifies the initial state of the multimodel view model.
// It checks that the model is initialized with the correct state and assignments based on the provided configuration.
func TestInitialMultimodelModel(t *testing.T) {
	cfg := &Config{
		Hosts: []Host{
			{
				Name:   "Test Host 1",
				URL:    "http://localhost:11434",
				Models: []string{"model1", "model2"},
			},
			{
				Name:   "Test Host 2",
				URL:    "http://localhost:11435",
				Models: []string{"model3", "model4"},
			},
		},
	}
	provider := newTestProvider()
	m := initialMultimodelModel(context.Background(), cfg, provider)

	if m.state != multimodelViewAssignment {
		t.Errorf("Expected initial state to be multimodelViewAssignment, got %v", m.state)
	}

	if len(m.assignments) != 2 {
		t.Errorf("Expected 2 assignments, got %d", len(m.assignments))
	}
}

// TestMultimodelUpdate exercises key transitions in the multimodel update loop.
// It simulates user interactions like navigation, model selection, and starting the chat,
// verifying that the model's state changes as expected.
func TestMultimodelUpdate(t *testing.T) {
	cfg := &Config{
		Hosts: []Host{
			{
				Name:   "Test Host 1",
				URL:    "http://localhost:11434",
				Models: []string{"model1", "model2"},
			},
		},
	}
	m := initialMultimodelModel(context.Background(), cfg, newTestProvider())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Expected a quit command, but got nil")
	}

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = newModel.(*multimodelModel)
	if m.selectedHostIndex != 0 {
		t.Errorf("Expected selectedHostIndex to be 0, got %d", m.selectedHostIndex)
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*multimodelModel)
	if !m.inModelSelection {
		t.Error("Expected inModelSelection to be true, but it's false")
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m = newModel.(*multimodelModel)
	if m.inModelSelection {
		t.Error("Expected inModelSelection to be false, but it's true")
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*multimodelModel)
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newModel.(*multimodelModel)
	if !m.assignments[0].isAssigned {
		t.Error("Expected model to be assigned, but it's not")
	}
	if m.assignments[0].selectedModel != "model1" {
		t.Errorf("Expected selected model to be 'model1', got '%s'", m.assignments[0].selectedModel)
	}

	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = newModel.(*multimodelModel)
	if m.state != multimodelViewLoadingChat {
		t.Errorf("Expected state to be multimodelViewLoadingChat, got %v", m.state)
	}
}

// TestMultimodelView ensures the multimodel view renders expected states.
// It checks that the view output changes correctly based on the model's state,
// including initialization, error display, and the assignment view.
func TestMultimodelView(t *testing.T) {
	cfg := &Config{
		Hosts: []Host{
			{
				Name:   "Test Host 1",
				URL:    "http://localhost:11434",
				Models: []string{"model1", "model2"},
			},
		},
	}
	m := initialMultimodelModel(context.Background(), cfg, newTestProvider())

	m.width = 0
	view := m.View()
	if view != "Initializing..." {
		t.Errorf("Expected view to be 'Initializing...', got '%s'", view)
	}

	m.width = 100
	m.err = multimodelChatReadyErr(errors.New("test error"))
	view = m.View()
	if !strings.Contains(view, "Error") {
		t.Errorf("Expected view to contain 'Error', got '%s'", view)
	}
	m.err = nil

	view = m.View()
	if !strings.Contains(view, "Multimodel Mode - Assign Models to Hosts") {
		t.Errorf("Expected view to contain 'Multimodel Mode - Assign Models to Hosts', got '%s'", view)
	}
}
