// cli/cli.go
// Package cli provides the command-line interface for the Agon application.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mwiater/agon/internal/appconfig"
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/viper"
)

// Config represents the shared application configuration for the CLI.
type Config = appconfig.Config

// Host represents a configured host entry within the application configuration.
type Host = appconfig.Host

// Parameters defines the configurable generation parameters for a language model on a host.
type Parameters = appconfig.Parameters

// LLMResponseMeta holds timing and tokenization metrics for a model response.
// This metadata is typically received in the final chunk of a streaming response
// and is displayed when debug mode is enabled.
type LLMResponseMeta struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

// chatMessage represents a single message in a chat conversation, containing the
// sender's role (e.g., "user", "assistant") and the message content.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// streamChunk represents a single chunk of a streaming language model response.
// It contains a portion of the message content and updated metadata.
type streamChunk struct {
	Model   string `json:"model"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done               bool  `json:"done"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

// ollamaTagsResponse represents the structure of the response from the Ollama
// /api/tags endpoint, which provides a list of available models.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// ollamaPsResponse represents the structure of the response from the Ollama
// /api/ps endpoint, which lists the models that are currently loaded in memory.
type ollamaPsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// viewState represents the current view or screen of the application.
type viewState int

const (
	viewHostSelector viewState = iota
	viewModelSelector
	viewLoadingChat
	viewChat
)

// model is the main application model for the Bubble Tea UI. It holds all the
// state necessary for the chat application to function.
type model struct {
	config           *Config
	client           *http.Client
	requestTimeout   time.Duration
	state            viewState
	isLoading        bool
	err              error
	hostList         list.Model
	modelList        list.Model
	textArea         textarea.Model
	viewport         viewport.Model
	spinner          spinner.Model
	chatHistory      []chatMessage
	responseBuf      strings.Builder
	responseMeta     LLMResponseMeta
	selectedHost     Host
	selectedModel    string
	loadedModels     []string
	width, height    int
	program          *tea.Program
	requestStartTime time.Time
}

// initialModel creates and initializes a new model with default values. It sets
// up the necessary Bubble Tea components, such as the spinner, textarea, and lists,
// and configures the HTTP client with the appropriate timeout.
func initialModel(cfg *Config) *model {
	timeout := cfg.RequestTimeout()
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()
	ta.Prompt = "Ask Anything: "
	ta.ShowLineNumbers = false
	ta.CharLimit = -1
	ta.SetHeight(1)
	ta.KeyMap.InsertNewline.SetEnabled(false)

	hostItems := make([]list.Item, len(cfg.Hosts))
	for i, h := range cfg.Hosts {
		hostItems[i] = item{title: h.Name, desc: h.URL}
	}
	hostDelegate := list.NewDefaultDelegate()
	hostList := list.New(hostItems, hostDelegate, 0, 0)
	hostList.Title = "Select a Host"

	vp := viewport.New(100, 5)

	return &model{
		config: cfg,
		client: &http.Client{
			Transport: &http.Transport{
				ForceAttemptHTTP2: false,
			},
			Timeout: timeout,
		},
		requestTimeout: timeout,
		state:          viewHostSelector,
		spinner:        s,
		textArea:       ta,
		hostList:       hostList,
		modelList:      list.New(nil, list.NewDefaultDelegate(), 0, 0),
		viewport:       vp,
	}
}

// item represents a selectable item in a Bubble Tea list. It is used for both
// hosts and models, and implements the list.Item interface.
type item struct {
	title  string
	desc   string
	loaded bool
}

// Title returns the title of the list item, satisfying the list.Item interface.
func (i item) Title() string { return i.title }

// Description returns the description of the list item. If the item represents a
// loaded model, it returns "Currently loaded". This satisfies the list.Item interface.
func (i item) Description() string {
	if i.loaded {
		return "Currently loaded"
	}
	return i.desc
}

// FilterValue returns the title of the item, which is used for filtering the list.
// This satisfies the list.Item interface.
func (i item) FilterValue() string { return i.title }

// modelsReadyMsg is a message sent when the list of models has been successfully
// fetched and processed.
type modelsReadyMsg struct {
	models       []list.Item
	loadedModels []string
}

// modelsLoadErr is a message sent when an error occurs while fetching the list of models.
type modelsLoadErr error

// chatReadyMsg is a message sent when the chat interface is ready for user interaction.
type chatReadyMsg struct{}

// chatReadyErr is a message sent when an error occurs while preparing the chat interface.
type chatReadyErr error

// streamChunkMsg is a message sent when a new chunk of a streaming response is received.
type streamChunkMsg string

// streamEndMsg is a message sent when a streaming response has completed.
type streamEndMsg struct{ meta LLMResponseMeta }

// streamErr is a message sent when an error occurs during a streaming response.
type streamErr error

// tickMsg is a message sent at regular intervals, used for animations and timed updates.
type tickMsg time.Time

// fetchAndSelectModelsCmd creates a Bubble Tea command that fetches the list of
// loaded models and then all available models for a given host. It prepares the
// model list for selection, prioritizing loaded models by placing them at the top.
func fetchAndSelectModelsCmd(host Host, client *http.Client, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		loadedModels, err := getLoadedModels(host, client, timeout)
		if err != nil {
			return modelsLoadErr(err)
		}

		allModels := host.Models

		loadedModelSet := make(map[string]struct{})
		for _, m := range loadedModels {
			loadedModelSet[m] = struct{}{}
		}

		var loadedItems []list.Item
		var otherItems []list.Item
		for _, m := range allModels {
			_, isLoaded := loadedModelSet[m]
			listItem := item{title: m, desc: "Select this model", loaded: isLoaded}
			if isLoaded {
				loadedItems = append(loadedItems, listItem)
			} else {
				otherItems = append(otherItems, listItem)
			}
		}

		finalModelItems := append(loadedItems, otherItems...)

		return modelsReadyMsg{
			models:       finalModelItems,
			loadedModels: loadedModels,
		}
	}
}

// getLoadedModels fetches the names of the models that are currently loaded into
// memory on the specified host by making a request to the /api/ps endpoint.
func getLoadedModels(host Host, client *http.Client, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", host.URL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var psResp ollamaPsResponse
	if err := json.Unmarshal(body, &psResp); err != nil {
		return nil, err
	}

	loadedModels := make([]string, len(psResp.Models))
	for i, m := range psResp.Models {
		loadedModels[i] = m.Name
	}
	return loadedModels, nil
}

// loadModelCmd creates a Bubble Tea command that attempts to load a specified
// model onto the given host. It does this by sending a minimal generate request
// to the /api/generate endpoint. This is typically used to ensure a model is
// ready for chat. It returns a tea.Msg indicating success (chatReadyMsg) or
// failure (chatReadyErr).
func loadModelCmd(host Host, modelName string, client *http.Client, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		payload := map[string]any{
			"model":  modelName,
			"prompt": ".",
			"stream": false,
		}
		body, _ := json.Marshal(payload)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", host.URL+"/api/generate", bytes.NewReader(body))
		if err != nil {
			return chatReadyErr(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return chatReadyErr(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return chatReadyErr(fmt.Errorf("API returned non-200 status: %s. Body: %s", resp.Status, string(bodyBytes)))
		}

		return chatReadyMsg{}
	}
}

// streamChatCmd creates a Bubble Tea command that initiates a streaming chat
// conversation with the selected language model. It sends the chat history and
// streams back the response chunk by chunk. It sends streamChunkMsg for each new
// chunk and a streamEndMsg when the stream is complete. Errors during streaming
// are sent as a streamErr message.
func streamChatCmd(p *tea.Program, host Host, modelName string, history []chatMessage, systemPrompt string, JSONFormat bool, parameters Parameters, client *http.Client, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		messages := history
		if systemPrompt != "" {
			messages = append([]chatMessage{{Role: "system", Content: systemPrompt}}, messages...)
		}

		payload := map[string]any{
			"model":    modelName,
			"messages": messages,
			"options":  parameters,
			"stream":   true,
		}

		if JSONFormat {
			payload = map[string]any{
				"model":    modelName,
				"messages": messages,
				"options":  parameters,
				"stream":   true,
				"format":   "json",
			}
		}

		body, _ := json.Marshal(payload)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		req, err := http.NewRequestWithContext(ctx, "POST", host.URL+"/api/chat", bytes.NewReader(body))
		if err != nil {
			cancel()
			return streamErr(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			return streamErr(err)
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancel()
			return streamErr(fmt.Errorf("API returned non-200 status: %s. Body: %s", resp.Status, string(bodyBytes)))
		}

		go func() {
			defer cancel()
			defer resp.Body.Close()
			decoder := json.NewDecoder(resp.Body)
			var finalChunk streamChunk
			for {
				var chunk streamChunk
				if err := decoder.Decode(&chunk); err != nil {
					if err != io.EOF {
						p.Send(streamErr(err))
					}
					break
				}
				p.Send(streamChunkMsg(chunk.Message.Content))
				if chunk.Done {
					finalChunk = chunk
					break
				}
			}
			p.Send(streamEndMsg{meta: LLMResponseMeta{
				Model:              finalChunk.Model,
				CreatedAt:          time.Now(),
				Done:               finalChunk.Done,
				TotalDuration:      finalChunk.TotalDuration,
				LoadDuration:       finalChunk.LoadDuration,
				PromptEvalCount:    finalChunk.PromptEvalCount,
				PromptEvalDuration: finalChunk.PromptEvalDuration,
				EvalCount:          finalChunk.EvalCount,
				EvalDuration:       finalChunk.EvalDuration,
			}})
		}()

		return nil
	}
}

// tickCmd creates a Bubble Tea command that sends a tickMsg at a regular interval.
// This is used to drive animations and other periodic UI updates.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Init initializes the Bubble Tea model and returns a command to start the spinner animation.
func (m *model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update is the central update function for the Bubble Tea model. It handles all
// incoming messages and updates the application's state accordingly. It is called
// by the Bubble Tea runtime when a message is received.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			if m.state == viewChat {
				m.state = viewHostSelector
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.hostList.SetSize(msg.Width-2, msg.Height-4)
		m.modelList.SetSize(msg.Width-2, msg.Height-4)
		m.textArea.SetWidth(msg.Width - 3)
		headerHeight := 4
		footerHeight := 5
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight

	case chatReadyMsg:
		m.isLoading = false
		m.state = viewChat
		m.textArea.Focus()
		m.viewport.GotoBottom()
		return m, nil

	case chatReadyErr:
		m.isLoading = false
		m.err = msg
		return m, nil

	case modelsReadyMsg:
		m.isLoading = false
		m.modelList.SetItems(msg.models)
		m.loadedModels = msg.loadedModels
		m.modelList.Title = fmt.Sprintf("Select a Model from %s", m.selectedHost.Name)
		m.state = viewModelSelector
		if len(m.loadedModels) > 0 {
			m.modelList.Select(0)
		}
		return m, nil

	case modelsLoadErr:
		m.isLoading = false
		m.err = msg
		return m, nil

	case streamChunkMsg:
		m.responseBuf.WriteString(string(msg))
		m.viewport.GotoBottom()
		return m, nil

	case streamEndMsg:
		m.responseMeta = msg.meta
		if m.responseBuf.Len() > 0 {
			m.chatHistory = append(m.chatHistory, chatMessage{
				Role:    "assistant",
				Content: m.responseBuf.String(),
			})
			m.responseBuf.Reset()
		}
		m.isLoading = false
		m.textArea.Focus()
		m.viewport.GotoBottom()
		return m, nil

	case streamErr:
		m.isLoading = false
		m.err = msg
		return m, nil
	case tickMsg:
		if m.isLoading {
			return m, tickCmd()
		}
		return m, nil
	}

	switch m.state {
	case viewHostSelector:
		m.hostList, cmd = m.hostList.Update(msg)
		cmds = append(cmds, cmd)
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			if _, ok := m.hostList.SelectedItem().(item); ok {
				m.selectedHost = m.config.Hosts[m.hostList.Index()]
				m.isLoading = true
				m.requestStartTime = time.Now()
				cmds = append(cmds, m.spinner.Tick, fetchAndSelectModelsCmd(m.selectedHost, m.client, m.requestTimeout), tickCmd())
			}
		}

	case viewModelSelector:
		m.modelList, cmd = m.modelList.Update(msg)
		cmds = append(cmds, cmd)
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			if selectedItem, ok := m.modelList.SelectedItem().(item); ok {
				m.selectedModel = selectedItem.Title()
				m.state = viewLoadingChat
				m.isLoading = true
				m.requestStartTime = time.Now()
				m.err = nil
				cmds = append(cmds, m.spinner.Tick, loadModelCmd(m.selectedHost, m.selectedModel, m.client, m.requestTimeout), tickCmd())
			}
		}

	case viewChat:
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		m.textArea, cmd = m.textArea.Update(msg)
		cmds = append(cmds, cmd)

		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			userInput := strings.TrimSpace(m.textArea.Value())
			if userInput != "" {
				m.responseMeta = LLMResponseMeta{}
				m.requestStartTime = time.Now()
				m.chatHistory = append(m.chatHistory, chatMessage{Role: "user", Content: userInput})
				m.textArea.Reset()
				m.isLoading = true
				m.err = nil

				cmds = append(cmds, m.spinner.Tick, streamChatCmd(m.program, m.selectedHost, m.selectedModel, m.chatHistory, m.selectedHost.SystemPrompt, m.config.JSONMode, m.selectedHost.Parameters, m.client, m.requestTimeout))
			}
		}
	}

	if m.isLoading {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the application's UI based on the current state of the model.
// It is called by the Bubble Tea runtime whenever the UI needs to be redrawn.
func (m *model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Padding(1)
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	switch m.state {
	case viewHostSelector, viewModelSelector:
		var listModel list.Model
		var title string
		if m.state == viewHostSelector {
			listModel = m.hostList
			title = m.hostList.Title
		} else {
			listModel = m.modelList
			title = m.modelList.Title
		}
		if m.isLoading {
			timer := fmt.Sprintf("%.1f", time.Since(m.requestStartTime).Seconds())
			return fmt.Sprintf("\n  %s Fetching models... %ss\n", m.spinner.View(), timer)
		}
		listView := listModel.View()
		if title != "" && !strings.Contains(listView, title) {
			listView = fmt.Sprintf("%s\n\n%s", title, listView)
		}
		return lipgloss.NewStyle().Margin(1, 2).Render(listView)

	case viewLoadingChat:
		timer := fmt.Sprintf("%.1f", time.Since(m.requestStartTime).Seconds())
		return fmt.Sprintf("\n  %s Loading %s... %ss\n", m.spinner.View(), m.selectedModel, timer)

	case viewChat:
		return m.chatView()

	default:
		return "Unknown state"
	}
}

// chatView renders the chat interface, including the header, chat history,
// current response (if streaming), and the input text area.
func (m *model) chatView() string {
	var builder strings.Builder

	headerStyle := lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Padding(0, 1)
	hostInfo := fmt.Sprintf("Host: %s", m.selectedHost.Name)
	modelInfo := fmt.Sprintf("Model: %s", m.selectedModel)

	var JSONMode string
	if viper.GetBool("debug") {
		JSONMode = fmt.Sprintf("JSON Mode: %s", "false")
	} else {
		JSONMode = fmt.Sprintf("JSON Mode: %s", "true")
	}

	var modelTopK string
	if m.selectedHost.Parameters.TopK != nil {
		modelTopK = fmt.Sprintf("TopK: %v", *m.selectedHost.Parameters.TopK)
	} else {
		modelTopK = "TopK: n/a"
	}

	var modelTopP string
	if m.selectedHost.Parameters.TopP != nil {
		modelTopP = fmt.Sprintf("TopP: %v", *m.selectedHost.Parameters.TopP)
	} else {
		modelTopP = "TopP: n/a"
	}

	var modelMinP string
	if m.selectedHost.Parameters.MinP != nil {
		modelMinP = fmt.Sprintf("MinP: %v", *m.selectedHost.Parameters.MinP)
	} else {
		modelMinP = "MinP: n/a"
	}

	var modelTFSZ string
	if m.selectedHost.Parameters.TFSZ != nil {
		modelTFSZ = fmt.Sprintf("TFSZ: %v", *m.selectedHost.Parameters.TFSZ)
	} else {
		modelTFSZ = "TFSZ: n/a"
	}

	var modelTypicalP string
	if m.selectedHost.Parameters.TypicalP != nil {
		modelTypicalP = fmt.Sprintf("TypicalP: %v", *m.selectedHost.Parameters.TypicalP)
	} else {
		modelTypicalP = "TypicalP: n/a"
	}

	var modelRepeatLastN string
	if m.selectedHost.Parameters.RepeatLastN != nil {
		modelRepeatLastN = fmt.Sprintf("RepeatLastN: %v", *m.selectedHost.Parameters.RepeatLastN)
	} else {
		modelRepeatLastN = "RepeatLastN: n/a"
	}

	var modelTemperature string
	if m.selectedHost.Parameters.Temperature != nil {
		modelTemperature = fmt.Sprintf("Temperature: %v", *m.selectedHost.Parameters.Temperature)
	} else {
		modelTemperature = "Temperature: n/a"
	}

	var modelRepeatPenalty string
	if m.selectedHost.Parameters.RepeatPenalty != nil {
		modelRepeatPenalty = fmt.Sprintf("RepeatPenalty: %v", *m.selectedHost.Parameters.RepeatPenalty)
	} else {
		modelRepeatPenalty = "RepeatPenalty: n/a"
	}

	var modelPresencePenalty string
	if m.selectedHost.Parameters.PresencePenalty != nil {
		modelPresencePenalty = fmt.Sprintf("PresencePenalty: %v", *m.selectedHost.Parameters.PresencePenalty)
	} else {
		modelPresencePenalty = "PresencePenalty: n/a"
	}

	var modelFrequencyPenalty string
	if m.selectedHost.Parameters.FrequencyPenalty != nil {
		modelFrequencyPenalty = fmt.Sprintf("FrequencyPenalty: %v", *m.selectedHost.Parameters.FrequencyPenalty)
	} else {
		modelFrequencyPenalty = "FrequencyPenalty: n/a"
	}

	var longestLength int

	modelStrings := []string{
		modelTopK,
		modelTopP,
		modelMinP,
		modelTFSZ,
		modelTypicalP,
		modelRepeatLastN,
		modelTemperature,
		modelRepeatPenalty,
		modelPresencePenalty,
		modelFrequencyPenalty,
	}

	for _, s := range modelStrings {
		length := len(s)
		if length > longestLength {
			longestLength = length
		}
	}

	labelString := "Config:"
	labelStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	jsonModeStyle := lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0")).Padding(0, 1).MarginLeft(1)
	paramStyle := lipgloss.NewStyle().Background(lipgloss.Color("0")).Foreground(lipgloss.Color("40")).Padding(0, 1).MarginLeft(1).MarginTop(1).Width(longestLength + 2)

	status := lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("Config:"),
		headerStyle.Render(hostInfo),
		headerStyle.MarginLeft(1).Render(modelInfo),
		jsonModeStyle.Render(JSONMode),
	)

	configSettingsLine1 := lipgloss.JoinHorizontal(lipgloss.Top,
		paramStyle.MarginLeft(len(labelString)+1).Render(modelTopK),
		paramStyle.Render(modelTopP),
		paramStyle.Render(modelMinP),
	)

	configSettingsLine2 := lipgloss.JoinHorizontal(lipgloss.Top,
		paramStyle.MarginLeft(len(labelString)+1).Render(modelTFSZ),
		paramStyle.Render(modelTypicalP),
		paramStyle.Render(modelRepeatLastN),
	)

	configSettingsLine3 := lipgloss.JoinHorizontal(lipgloss.Top,
		paramStyle.MarginLeft(len(labelString)+1).Render(modelTemperature),
		paramStyle.Render(modelRepeatPenalty),
		paramStyle.Render(modelPresencePenalty),
	)

	configSettingsLine4 := lipgloss.JoinHorizontal(lipgloss.Top,
		paramStyle.MarginLeft(len(labelString)+1).Render(modelFrequencyPenalty),
	)

	help := lipgloss.NewStyle().Render(" (tab to change, esc to quit)")
	builder.WriteString(status + help + configSettingsLine1 + configSettingsLine2 + configSettingsLine3 + configSettingsLine4 + "\n\n")

	var historyBuilder strings.Builder
	userStyle := lipgloss.NewStyle().Bold(true)
	assistantStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))

	for _, msg := range m.chatHistory {
		var role, content string
		if msg.Role == "assistant" {
			role = assistantStyle.Render("Assistant: ")
			content = msg.Content
		} else {
			role = userStyle.Render("You: ")
			content = msg.Content
		}
		wrappedContent := lipgloss.NewStyle().Width(m.width - lipgloss.Width(role) - 2).Render(content)
		historyBuilder.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, role, wrappedContent) + "\n")
	}

	if m.responseBuf.Len() > 0 {
		role := assistantStyle.Render("Assistant: ")
		wrappedContent := lipgloss.NewStyle().Width(m.width - lipgloss.Width(role) - 2).Render(m.responseBuf.String())
		historyBuilder.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, role, wrappedContent))
	}

	m.viewport.SetContent(historyBuilder.String())
	builder.WriteString(m.viewport.View())

	if m.isLoading {
		timer := fmt.Sprintf("%.1f", time.Since(m.requestStartTime).Seconds())
		loadingText := fmt.Sprintf(" Assistant is thinking... %ss", timer)
		builder.WriteString("\n" + m.spinner.View() + loadingText)
	} else {
		builder.WriteString("\n" + m.textArea.View())
	}

	if m.config.Debug && m.responseMeta.Done {
		builder.WriteString("\n" + formatMeta(m.responseMeta))
	}

	return builder.String()
}

// formatMeta formats the LLMResponseMeta into a human-readable string,
// displaying various performance metrics of the language model response.
func formatMeta(meta LLMResponseMeta) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	loadDur := float64(meta.LoadDuration) / 1e9
	promptEvalDur := float64(meta.PromptEvalDuration) / 1e9
	evalDur := float64(meta.EvalDuration) / 1e9
	totalDur := float64(meta.TotalDuration) / 1e9

	return style.Render(fmt.Sprintf(
		"  >>> [Model Load Duration: %.1fs] [Prompt Eval: %.1fs | %d Tokens] [Response Eval: %.1fs | %d Tokens] [Total Duration: %.1fs]",
		loadDur,
		promptEvalDur,
		meta.PromptEvalCount,
		evalDur,
		meta.EvalCount,
		totalDur,
	))
}

// StartGUI initializes and runs the interactive TUI for single-model chat.
// It reads configuration from config/config.json by default, optionally switches to multimodel
// mode, and blocks until the UI exits. It logs diagnostic output to debug.log
// when enabled. StartGUI does not return a value.
func StartGUI(cfg *appconfig.Config) {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		log.Fatalf("could not open log file: %v", err)
	}
	defer f.Close()

	if cfg == nil {
		log.Fatalf("Failed to start: configuration is not loaded")
	}

	if cfg.MultimodelMode {
		models.UnloadModels(cfg)
		if err := StartMultimodelGUI(cfg); err != nil {
			log.Fatalf("Error running multimodel program: %v", err)
		}
		return
	}

	m := initialModel(cfg)

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}
