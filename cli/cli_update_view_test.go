// cli/cli_update_view_test.go
package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// TestSingleModel_StateTransitions_And_View covers the single-model state machine and view rendering.
// It verifies that the UI transitions correctly between host selection, model selection, loading,
// and chat views, and that chat messages are processed and displayed as expected.
func TestSingleModel_StateTransitions_And_View(t *testing.T) {
	cfg := &Config{Hosts: []Host{{Name: "HostA", URL: "http://x", Models: []string{"m1", "m2"}}}}
	provider := newTestProvider()
	provider.loadedModels["HostA"] = []string{"m1"}
	m := initialModel(context.Background(), cfg, provider)

	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(*model)
	if !m.isLoading || m.state != viewHostSelector {
		t.Fatalf("expected loading host selector; got loading=%v state=%v", m.isLoading, m.state)
	}

	items := []list.Item{item{title: "m1", desc: "Select this model", loaded: true}, item{title: "m2", desc: "Select this model"}}
	m2, _ = m.Update(modelsReadyMsg{models: items, loadedModels: []string{"m1"}})
	m = m2.(*model)
	if m.state != viewModelSelector || len(m.modelList.Items()) != 2 {
		t.Fatalf("expected model selector with 2 items; state=%v count=%d", m.state, len(m.modelList.Items()))
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(*model)
	if !m.isLoading || m.state != viewLoadingChat {
		t.Fatalf("expected loading chat; got loading=%v state=%v", m.isLoading, m.state)
	}

	m2, _ = m.Update(chatReadyMsg{})
	m = m2.(*model)
	if m.state != viewChat {
		t.Fatalf("expected chat view; got %v", m.state)
	}

	m.textArea.SetValue("hello")
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(*model)
	if len(m.chatHistory) == 0 || m.chatHistory[len(m.chatHistory)-1].Role != "user" {
		t.Fatalf("expected last message to be user; history=%v", m.chatHistory)
	}
	if !m.isLoading {
		t.Fatalf("expected loading after sending message")
	}

	m2, _ = m.Update(streamChunkMsg("world"))
	m = m2.(*model)
	if !strings.Contains(m.responseBuf.String(), "world") {
		t.Fatalf("expected response buffer to contain chunk")
	}
	m2, _ = m.Update(streamEndMsg{meta: LLMResponseMeta{Done: true}})
	m = m2.(*model)
	if m.isLoading {
		t.Fatalf("expected not loading after stream end")
	}
	if len(m.chatHistory) < 2 || m.chatHistory[len(m.chatHistory)-1].Role != "assistant" {
		t.Fatalf("expected assistant message after end; history=%v", m.chatHistory)
	}

	out := m.View()
	if !strings.Contains(out, "Assistant:") || !strings.Contains(out, "You:") {
		t.Fatalf("expected roles in view output; got: %s", out)
	}
}

// TestMultimodel_Assignment_And_Chat_Flow validates the multimodel assignment and chat flow.
// It simulates host and model selection, verifies that assignments are correctly made,
// and checks the chat interaction within the multimodel interface.
func TestMultimodel_Assignment_And_Chat_Flow(t *testing.T) {
	cfg := &Config{Hosts: []Host{
		{Name: "H1", URL: "http://x", Models: []string{"m1", "m2"}},
		{Name: "H2", URL: "http://y", Models: []string{"m3"}},
		{Name: "H3", URL: "http://z", Models: []string{"m4"}},
		{Name: "H4", URL: "http://w", Models: []string{"m5"}},
	}}
	provider := newTestProvider()

	mm := initialMultimodelModel(context.Background(), cfg, provider)
	mm.program = &tea.Program{}

	_, _ = mm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m2, _ := mm.updateAssignment(tea.KeyMsg{Type: tea.KeyEnter})
	mm = m2.(*multimodelModel)
	if !mm.inModelSelection || len(mm.modelList.Items()) == 0 {
		t.Fatalf("expected to be in model selection with items")
	}

	m2, _ = mm.updateAssignment(tea.KeyMsg{Type: tea.KeyEnter})
	mm = m2.(*multimodelModel)
	if !mm.assignments[mm.selectedHostIndex].isAssigned {
		t.Fatalf("expected assignment to be marked assigned")
	}

	m2, _ = mm.updateAssignment(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mm = m2.(*multimodelModel)
	if mm.state != multimodelViewLoadingChat {
		t.Fatalf("expected loading chat state; got %v", mm.state)
	}

	m2, _ = mm.Update(multimodelChatReadyMsg{})
	mm = m2.(*multimodelModel)
	if mm.state != multimodelViewChat {
		t.Fatalf("expected chat view; got %v", mm.state)
	}

	mm.textArea.SetValue("question")
	m2, _ = mm.updateChat(tea.KeyMsg{Type: tea.KeyEnter})
	mm = m2.(*multimodelModel)
	if !mm.columnResponses[0].isStreaming {
		t.Fatalf("expected first column streaming")
	}

	m2, _ = mm.Update(multimodelStreamChunkMsg{hostIndex: 0, message: chatMessage{Role: "assistant", Content: "hi"}})
	mm = m2.(*multimodelModel)
	if len(mm.columnResponses[0].chatHistory) == 0 {
		t.Fatalf("expected assistant message in column history")
	}
	m2, _ = mm.Update(multimodelStreamEndMsg{hostIndex: 0, meta: LLMResponseMeta{Done: true}})
	mm = m2.(*multimodelModel)
	if mm.isLoading {
		t.Fatalf("expected not loading after all streams ended")
	}

	out := mm.multimodelChatView()
	if !strings.Contains(out, "Multimodel Chat") {
		t.Fatalf("expected chat header in view; got: %s", out)
	}
}
