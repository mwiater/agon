// cli/cli_jsonmode_integration_test.go
package cli

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/providers"
)

type stubChatProvider struct {
	reqCh chan providers.StreamRequest
}

func (s *stubChatProvider) LoadedModels(ctx context.Context, host appconfig.Host) ([]string, error) {
	return nil, nil
}

func (s *stubChatProvider) EnsureModelReady(ctx context.Context, host appconfig.Host, model string) error {
	return nil
}

func (s *stubChatProvider) Stream(ctx context.Context, req providers.StreamRequest, callbacks providers.StreamCallbacks) error {
	s.reqCh <- req
	if callbacks.OnComplete != nil {
		_ = callbacks.OnComplete(providers.StreamMetadata{Done: true})
	}
	return nil
}

func (s *stubChatProvider) Close() error { return nil }

type dummyModel struct{}

func (d dummyModel) Init() tea.Cmd                       { return nil }
func (d dummyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return d, nil }
func (d dummyModel) View() string                        { return "" }

func TestStreamChatCmdPassesJSONMode(t *testing.T) {
	reqCh := make(chan providers.StreamRequest, 1)
	provider := &stubChatProvider{reqCh: reqCh}

	p := tea.NewProgram(dummyModel{})

	cmd := streamChatCmd(context.Background(), p, provider, Host{Name: "Host01", URL: "http://example"}, "model1", nil, "", true, Parameters{})
	cmd()

	select {
	case req := <-reqCh:
		if !req.JSONMode {
			t.Fatal("expected JSONMode=true in stream request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream request")
	}
}
