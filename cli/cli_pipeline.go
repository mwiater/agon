// cli/cli_pipeline.go
// Package cli contains the interactive terminal interfaces for Agon, including
// the pipeline mode UI defined in this file.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mwiater/agon/internal/logging"
	"github.com/mwiater/agon/internal/providerfactory"
	"github.com/mwiater/agon/internal/providers"
	"github.com/mwiater/agon/internal/providers/ollama"
	"github.com/mwiater/agon/internal/util"
)

const (
	pipelineStageCount       = 4
	pipelineMaxHandoffTokens = 4096
	pipelinePreviewRunes     = 120
)

var (
	stageTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	stageBadgeStyle   = lipgloss.NewStyle().Faint(true)
	stageModelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	stageStatusStyles = map[pipelineStageStatus]lipgloss.Style{
		pipelineStageStatusUnassigned: lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("235")).Padding(0, 1),
		pipelineStageStatusWaiting:    lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("238")).Padding(0, 1),
		pipelineStageStatusRunning:    lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("33")).Padding(0, 1),
		pipelineStageStatusDone:       lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("34")).Padding(0, 1),
		pipelineStageStatusSkipped:    lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(lipgloss.Color("236")).Padding(0, 1),
		pipelineStageStatusError:      lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1),
	}
	stageCacheStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	focusedColumn   = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("135")).Padding(0, 1)
	normalColumn    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	overlayStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1).Width(80)
	bannerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("124")).Padding(0, 1)
)

// pipelineViewState describes which primary view is active inside pipeline mode.
type pipelineViewState int

const (
	pipelineViewAssignment pipelineViewState = iota
	pipelineViewReady
	pipelineViewRunning
	pipelineViewExpanded
)

// pipelineStageStatus enumerates the lifecycle states for an individual stage.
type pipelineStageStatus int

const (
	pipelineStageStatusUnassigned pipelineStageStatus = iota
	pipelineStageStatusWaiting
	pipelineStageStatusRunning
	pipelineStageStatusDone
	pipelineStageStatusSkipped
	pipelineStageStatusError
)

// pipelineStageView determines which content panel a stage is rendering.
type pipelineStageView int

const (
	pipelineStageViewOutput pipelineStageView = iota
	pipelineStageViewStats
	pipelineStageViewHandoff
)

// pipelineHandoffMode identifies how a stage hands off data to the next stage.
type pipelineHandoffMode int

const (
	pipelineHandoffRaw pipelineHandoffMode = iota
	pipelineHandoffSelector
	pipelineHandoffTemplate
)

// pipelineHandoff contains the payload and metadata shared with downstream stages.
type pipelineHandoff struct {
	mode              pipelineHandoffMode
	payload           string
	preview           string
	truncated         bool
	truncationSummary string
	redactions        []string
	tokenCount        int
}

// hostSelectorItem renders hosts inside the assignment picker.
type hostSelectorItem struct {
	index int
	host  Host
}

func (i hostSelectorItem) Title() string       { return i.host.Name }
func (i hostSelectorItem) Description() string { return i.host.URL }
func (i hostSelectorItem) FilterValue() string { return i.host.Name }

// modelSelectorItem renders models inside the assignment picker.
type modelSelectorItem struct {
	name string
}

func (i modelSelectorItem) Title() string       { return i.name }
func (i modelSelectorItem) Description() string { return "Select model" }
func (i modelSelectorItem) FilterValue() string { return i.name }

// pipelineStage stores per-stage assignment, execution state, and cached results.
type pipelineStage struct {
	index           int
	host            Host
	hostIndex       int
	hasAssignment   bool
	selectedModel   string
	availableModels []string
	parameters      Parameters
	systemPrompt    string

	status        pipelineStageStatus
	statusMessage string
	view          pipelineStageView

	outputBuffer strings.Builder
	finalOutput  string
	stats        LLMResponseMeta
	cacheHit     bool

	startedAt   time.Time
	firstToken  time.Time
	completedAt time.Time

	history []chatMessage
	handoff pipelineHandoff
}

// pipelineCacheEntry memoizes a stage response for reuse within the session.
type pipelineCacheEntry struct {
	output    string
	meta      LLMResponseMeta
	handoff   pipelineHandoff
	timestamp time.Time
}

// pipelineExportRecord captures per-stage export data.
type pipelineExportRecord struct {
	Stage             int           `json:"stage"`
	Host              string        `json:"host"`
	Model             string        `json:"model"`
	Parameters        Parameters    `json:"parameters"`
	SystemPromptHash  string        `json:"systemPromptHash"`
	Timings           exportTimings `json:"timings"`
	Tokens            exportTokens  `json:"tokens"`
	OutputHash        string        `json:"outputHash"`
	HandoffPayload    string        `json:"handoff"`
	CacheHit          bool          `json:"cacheHit"`
	TruncationSummary string        `json:"truncationSummary,omitempty"`
}

type exportTimings struct {
	TotalSeconds      float64 `json:"totalSeconds"`
	LoadSeconds       float64 `json:"loadSeconds"`
	PromptEvalSeconds float64 `json:"promptEvalSeconds"`
	EvalSeconds       float64 `json:"evalSeconds"`
	TimeToFirstToken  float64 `json:"timeToFirstToken"`
}

type exportTokens struct {
	Prompt int `json:"prompt"`
	Eval   int `json:"eval"`
}

// pipelineModel owns all state for the pipeline Bubble Tea program.
type pipelineModel struct {
	ctx            context.Context
	config         *Config
	client         *http.Client
	requestTimeout time.Duration
	mcpStatus      mcpStatus
	provider       providers.ChatProvider

	viewState     pipelineViewState
	focusIndex    int
	expandedIndex int

	stages      []pipelineStage
	stageInputs [pipelineStageCount]string

	spinner   spinner.Model
	textArea  textarea.Model
	viewport  viewport.Model
	hostList  list.Model
	modelList list.Model

	selectingHost  bool
	selectingModel bool
	selectedStage  int

	width, height int
	program       *tea.Program

	statusBanner  string
	runInProgress bool

	requestStartTime time.Time

	showHandoffOverlay bool
	overlayStageIndex  int

	memoCache map[string]pipelineCacheEntry

	exportRecords      []pipelineExportRecord
	exportPath         string
	exportMarkdownPath string
	runStarted         time.Time
	runCompleted       time.Time

	switchToMultimodel bool

	// remember selections to streamline assignment UX
	nextHostIndex      int
	defaultModelByHost map[string]string // key: host URL, value: model name
	globalDefaultModel string            // model selected for Stage 1, used as default elsewhere
}

// initialPipelineModel constructs a model with sensible defaults and four stages.
func initialPipelineModel(ctx context.Context, cfg *Config, provider providers.ChatProvider) *pipelineModel {
	timeout := cfg.RequestTimeout()

	s := spinner.New()
	s.Spinner = spinner.Dot

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Prompt = "Ask Anything: "
	ta.ShowLineNumbers = false
	ta.CharLimit = -1
	ta.SetHeight(1)
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(100, 5)

	stages := make([]pipelineStage, pipelineStageCount)
	for i := range stages {
		stages[i] = pipelineStage{
			index:  i,
			view:   pipelineStageViewOutput,
			status: pipelineStageStatusUnassigned,
			handoff: pipelineHandoff{
				mode: pipelineHandoffRaw,
			},
		}
	}

	hostItems := make([]list.Item, len(cfg.Hosts))
	for i, host := range cfg.Hosts {
		hostItems[i] = hostSelectorItem{index: i, host: host}
	}
	hostList := list.New(hostItems, list.NewDefaultDelegate(), 0, 0)
	hostList.Title = "Select a Host"

	modelList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	modelList.Title = "Select a Model"

	return &pipelineModel{
		ctx:                ctx,
		config:             cfg,
		requestTimeout:     timeout,
		mcpStatus:          deriveMCPStatus(cfg, provider),
		provider:           provider,
		viewState:          pipelineViewAssignment,
		focusIndex:         0,
		expandedIndex:      -1,
		stages:             stages,
		spinner:            s,
		textArea:           ta,
		viewport:           vp,
		hostList:           hostList,
		modelList:          modelList,
		selectedStage:      0,
		overlayStageIndex:  -1,
		memoCache:          make(map[string]pipelineCacheEntry),
		exportPath:         cfg.ExportPath,
		exportMarkdownPath: cfg.ExportMarkdownPath,
		nextHostIndex:      0,
		defaultModelByHost: make(map[string]string),
	}
}

// Init satisfies the tea.Model interface.
func (m *pipelineModel) Init() tea.Cmd {
	return nil
}

// pipeline message types emitted during execution.
type (
	pipelineStageChunkMsg struct {
		Stage   int
		Content string
	}
	pipelineStageDoneMsg struct {
		Stage  int
		Output string
		Meta   LLMResponseMeta
	}
	pipelineStageErrorMsg struct {
		Stage int
		Err   error
	}
	pipelineStageCacheHitMsg struct {
		Stage int
		Entry pipelineCacheEntry
	}
)

// Update routes incoming messages to the appropriate handlers.
func (m *pipelineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.hostList.SetSize(msg.Width-2, msg.Height-6)
		m.modelList.SetSize(msg.Width-2, msg.Height-6)
		m.textArea.SetWidth(msg.Width - 3)
		headerHeight := 4
		footerHeight := 5
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
		return m, nil

	case pipelineStageChunkMsg:
		m.handleStageChunk(msg)
		return m, nil

	case pipelineStageDoneMsg:
		cmd := m.handleStageDone(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case pipelineStageErrorMsg:
		m.handleStageError(msg)
		return m, nil

	case pipelineStageCacheHitMsg:
		cmd := m.handleStageCacheHit(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd

	switch m.viewState {
	case pipelineViewAssignment:
		cmd = m.updateAssignment(msg)
	case pipelineViewReady, pipelineViewRunning:
		cmd = m.updateActive(msg)
	case pipelineViewExpanded:
		cmd = m.updateExpanded(msg)
	}

	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if m.runInProgress {
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		if spinCmd != nil {
			cmds = append(cmds, spinCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// updateAssignment manages the host/model selection workflow.
func (m *pipelineModel) updateAssignment(msg tea.Msg) tea.Cmd {
	if m.selectingHost {
		var cmd tea.Cmd
		m.hostList, cmd = m.hostList.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				if item, ok := m.hostList.SelectedItem().(hostSelectorItem); ok {
					stage := &m.stages[m.selectedStage]
					stage.host = item.host
					stage.hostIndex = item.index
					stage.availableModels = append([]string(nil), item.host.Models...)
					stage.parameters = item.host.Parameters
					stage.systemPrompt = item.host.SystemPrompt
					stage.hasAssignment = false
					stage.selectedModel = ""
					stage.status = pipelineStageStatusUnassigned
					stage.statusMessage = ""

					m.modelList.SetItems(nil)
					modelItems := make([]list.Item, len(stage.availableModels))
					for i, model := range stage.availableModels {
						modelItems[i] = modelSelectorItem{name: model}
					}
					m.modelList.SetItems(modelItems)
					if len(modelItems) > 0 {
						// Prefer globally chosen default model (Stage 1), then host-specific default
						sel := 0
						if m.globalDefaultModel != "" {
							for i, it := range stage.availableModels {
								if it == m.globalDefaultModel {
									sel = i
									break
								}
							}
						} else if def, ok := m.defaultModelByHost[stage.host.URL]; ok {
							for i, it := range stage.availableModels {
								if it == def {
									sel = i
									break
								}
							}
						}
						m.modelList.Select(sel)
					}
					m.selectingHost = false
					m.selectingModel = len(modelItems) > 0
				}
			case "esc":
				m.selectingHost = false
			}
		}
		return cmd
	}

	if m.selectingModel {
		var cmd tea.Cmd
		m.modelList, cmd = m.modelList.Update(msg)
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				if item, ok := m.modelList.SelectedItem().(modelSelectorItem); ok {
					stage := &m.stages[m.selectedStage]
					stage.selectedModel = item.name
					stage.hasAssignment = true
					stage.status = pipelineStageStatusWaiting
					stage.statusMessage = "Ready"
					stage.view = pipelineStageViewOutput
					stage.outputBuffer.Reset()
					stage.finalOutput = ""
					m.selectingModel = false

					// Record default model for this host (first selection becomes default)
					if stage.host.URL != "" {
						if _, exists := m.defaultModelByHost[stage.host.URL]; !exists {
							m.defaultModelByHost[stage.host.URL] = stage.selectedModel
						}
					}

					// If this is the first stage and no global default set, remember it
					if m.selectedStage == 0 && m.globalDefaultModel == "" {
						m.globalDefaultModel = stage.selectedModel
					}

					// Advance default host selection to the next host for convenience
					if len(m.config.Hosts) > 0 {
						m.nextHostIndex = (stage.hostIndex + 1) % len(m.config.Hosts)
					}

					// Move cursor to next stage to streamline workflow
					if m.selectedStage < pipelineStageCount-1 {
						m.selectedStage++
					}
				}
			case "esc":
				m.selectingModel = false
			}
		}
		return cmd
	}

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+c", "q":
			return tea.Quit
		case "up", "k":
			if m.selectedStage > 0 {
				m.selectedStage--
			}
		case "down", "j":
			if m.selectedStage < pipelineStageCount-1 {
				m.selectedStage++
			}
		case "enter", "h":
			if len(m.config.Hosts) == 0 {
				m.statusBanner = "No hosts configured"
				return nil
			}
			// Preselect the next host to streamline assignments
			if len(m.config.Hosts) > 0 {
				idx := m.nextHostIndex
				if idx < 0 || idx >= len(m.config.Hosts) {
					idx = 0
				}
				m.hostList.Select(idx)
			}
			m.selectingHost = true
			return nil
		case "m":
			stage := &m.stages[m.selectedStage]
			if !stage.hasAssignment {
				m.statusBanner = "Select a host before choosing a model"
				return nil
			}
			modelItems := make([]list.Item, len(stage.availableModels))
			for i, model := range stage.availableModels {
				modelItems[i] = modelSelectorItem{name: model}
			}
			m.modelList.SetItems(modelItems)
			if len(modelItems) > 0 {
				// Prefer global default (from Stage 1) else host-specific default if known
				sel := 0
				if m.globalDefaultModel != "" {
					for i, it := range stage.availableModels {
						if it == m.globalDefaultModel {
							sel = i
							break
						}
					}
				} else if def, ok := m.defaultModelByHost[stage.host.URL]; ok {
					for i, it := range stage.availableModels {
						if it == def {
							sel = i
							break
						}
					}
				}
				m.modelList.Select(sel)
				m.selectingModel = true
			}
		case "d":
			stage := &m.stages[m.selectedStage]
			stage.host = Host{}
			stage.hasAssignment = false
			stage.selectedModel = ""
			stage.status = pipelineStageStatusUnassigned
			stage.statusMessage = ""
			stage.availableModels = nil
			stage.history = nil
			stage.outputBuffer.Reset()
			stage.finalOutput = ""
		case "c":
			if !m.anyStageAssigned() {
				m.statusBanner = "Assign at least one stage before starting the pipeline"
				return nil
			}
			if err := m.preflightAssignments(); err != nil {
				m.statusBanner = err.Error()
				return nil
			}
			m.viewState = pipelineViewReady
			m.focusIndex = m.firstAssignedStage()
			if m.focusIndex == -1 {
				m.focusIndex = 0
			}
			m.textArea.Focus()
			m.statusBanner = ""
			return nil
		}
	}

	return nil
}

// updateActive handles interactions while the pipeline view is visible.
func (m *pipelineModel) updateActive(msg tea.Msg) tea.Cmd {
	textFocused := m.textArea.Focused()

	switch km := msg.(type) {
	case tea.KeyMsg:
		switch km.String() {
		case "ctrl+c", "ctrl+q":
			return tea.Quit
		case "left":
			if !textFocused {
				m.moveFocus(-1)
			}
		case "right":
			if !textFocused {
				m.moveFocus(1)
			}
		case "ctrl+h", "ctrl+left":
			m.moveFocus(-1)
		case "ctrl+l", "ctrl+right":
			m.moveFocus(1)
		case "ctrl+s":
			if !textFocused {
				stage := &m.stages[m.focusIndex]
				stage.view = (stage.view + 1) % 3
				if stage.view != pipelineStageViewHandoff {
					m.showHandoffOverlay = false
					m.overlayStageIndex = -1
				}
			}
		case "ctrl+o":
			stage := &m.stages[m.focusIndex]
			if stage.view == pipelineStageViewHandoff {
				if m.showHandoffOverlay && m.overlayStageIndex == m.focusIndex {
					m.showHandoffOverlay = false
					m.overlayStageIndex = -1
				} else {
					m.showHandoffOverlay = true
					m.overlayStageIndex = m.focusIndex
				}
			}
		case "tab":
			if textFocused {
				break
			}
			stage := &m.stages[m.focusIndex]
			if stage.view == pipelineStageViewHandoff {
				if m.showHandoffOverlay && m.overlayStageIndex == m.focusIndex {
					m.showHandoffOverlay = false
					m.overlayStageIndex = -1
				} else {
					m.showHandoffOverlay = true
					m.overlayStageIndex = m.focusIndex
				}
			}
		case "enter":
			if m.textArea.Focused() && !m.runInProgress {
				input := strings.TrimSpace(m.textArea.Value())
				if input != "" {
					return m.startPipelineRun(input)
				}
			}
		case "ctrl+enter":
			if m.viewState == pipelineViewExpanded {
				m.viewState = pipelineViewReady
				m.expandedIndex = -1
			} else if !textFocused {
				m.viewState = pipelineViewExpanded
				m.expandedIndex = m.focusIndex
			}
		case "esc":
			if m.showHandoffOverlay {
				m.showHandoffOverlay = false
				m.overlayStageIndex = -1
				return nil
			}
			if m.viewState == pipelineViewExpanded {
				m.viewState = pipelineViewReady
				m.expandedIndex = -1
			} else if m.runInProgress {
				// ignore
			} else {
				m.textArea.Blur()
			}
		case "ctrl+p":
			m.switchToMultimodel = true
			return tea.Quit
		case "ctrl+e":
			if len(m.exportRecords) == 0 {
				m.statusBanner = "Run the pipeline before exporting"
				return nil
			}
			if m.runCompleted.IsZero() {
				m.runCompleted = time.Now()
			}
			jsonPath := strings.TrimSpace(m.exportPath)
			if jsonPath == "" {
				jsonPath = "pipeline.json"
			}
			var notices []string
			if err := m.exportPipelineJSON(jsonPath); err != nil {
				notices = append(notices, fmt.Sprintf("JSON export failed: %v", err))
			} else {
				notices = append(notices, fmt.Sprintf("JSON → %s", jsonPath))
			}
			if markdownPath := strings.TrimSpace(m.exportMarkdownPath); markdownPath != "" {
				if err := m.exportPipelineMarkdown(markdownPath); err != nil {
					notices = append(notices, fmt.Sprintf("Markdown export failed: %v", err))
				} else {
					notices = append(notices, fmt.Sprintf("Markdown → %s", markdownPath))
				}
			}
			m.statusBanner = strings.Join(notices, " | ")
		}
	}

	if m.viewState != pipelineViewExpanded {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		if cmd != nil {
			return cmd
		}
	}

	return nil
}

// updateExpanded processes input while a stage is expanded to full view.
func (m *pipelineModel) updateExpanded(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc", "enter", "ctrl+enter":
			m.viewState = pipelineViewReady
			m.expandedIndex = -1
		case "left":
			m.moveFocus(-1)
			m.expandedIndex = m.focusIndex
		case "right":
			m.moveFocus(1)
			m.expandedIndex = m.focusIndex
		case "ctrl+h", "ctrl+left":
			m.moveFocus(-1)
			m.expandedIndex = m.focusIndex
		case "ctrl+l", "ctrl+right":
			m.moveFocus(1)
			m.expandedIndex = m.focusIndex
		case "ctrl+s":
			stage := &m.stages[m.focusIndex]
			stage.view = (stage.view + 1) % 3
		case "ctrl+o":
			stage := &m.stages[m.focusIndex]
			if stage.view == pipelineStageViewHandoff {
				if m.showHandoffOverlay && m.overlayStageIndex == m.focusIndex {
					m.showHandoffOverlay = false
					m.overlayStageIndex = -1
				} else {
					m.showHandoffOverlay = true
					m.overlayStageIndex = m.focusIndex
				}
			}
		}
	}
	return nil
}

// View renders the current pipeline view.
func (m *pipelineModel) View() string {
	if m.width == 0 {
		return "Initializing pipeline mode..."
	}

	switch m.viewState {
	case pipelineViewAssignment:
		return m.assignmentView()
	case pipelineViewReady, pipelineViewRunning:
		return m.pipelineView()
	case pipelineViewExpanded:
		return m.expandedView()
	default:
		return "Unknown pipeline state"
	}
}

func (m *pipelineModel) assignmentView() string {
	var builder strings.Builder

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render("Pipeline Mode - Map hosts to stages")
	builder.WriteString(header + "\n")
	builder.WriteString(renderMCPBadge(m.mcpStatus) + "\n\n")

	for i, stage := range m.stages {
		pointer := "  "
		if i == m.selectedStage {
			pointer = "> "
		}
		builder.WriteString(pointer)

		stageLabel := fmt.Sprintf("Stage %d", i+1)
		builder.WriteString(stageTitleStyle.Render(stageLabel))
		builder.WriteString(" → ")

		if stage.hasAssignment {
			badge := fmt.Sprintf("%s • %s", stage.host.Name, stage.selectedModel)
			builder.WriteString(stageModelStyle.Render(badge))
		} else if stage.host.URL != "" {
			builder.WriteString(stageBadgeStyle.Render(fmt.Sprintf("%s • (select model)", stage.host.Name)))
		} else {
			builder.WriteString(stageBadgeStyle.Render("(no host)"))
		}

		if stage.statusMessage != "" {
			builder.WriteString("  ")
			builder.WriteString(stageStatusStyles[stage.status].Render(stage.statusMessage))
		}

		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	help := "↑/↓ select stage  Enter/h pick host  m pick model  d clear  c continue  q quit"
	if m.statusBanner != "" {
		builder.WriteString(bannerStyle.Render(m.statusBanner) + "\n")
	}
	builder.WriteString(lipgloss.NewStyle().Faint(true).Render(help))

	if m.selectingHost {
		return lipgloss.NewStyle().Margin(1, 2).Render(m.hostList.View())
	}
	if m.selectingModel {
		return lipgloss.NewStyle().Margin(1, 2).Render(m.modelList.View())
	}

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func (m *pipelineModel) pipelineView() string {
	var parts []string

	parts = append(parts, m.renderProgressLine())
	if m.statusBanner != "" {
		parts = append(parts, bannerStyle.Render(m.statusBanner))
	}
	// Calculate a target height for the stage columns to fill the console
	partsAbove := 1 // progress line
	if m.statusBanner != "" {
		partsAbove++ // banner
	}
	partsBelow := 2 // input/spinner + help
	separators := 3 // blank lines between progress-columns, columns-input, input-help
	if m.statusBanner != "" {
		separators++ // progress-banner and banner-columns
	}
	marginTop, marginBottom := 1, 1 // outer margin in View()
	available := m.height - marginTop - marginBottom - partsAbove - partsBelow - separators
	// Account for borders (top+bottom) added by each column wrapper
	available -= 2
	if available < 3 {
		available = 3
	}
	parts = append(parts, m.renderStageColumns(available))

	if m.showHandoffOverlay && m.overlayStageIndex >= 0 && m.overlayStageIndex < len(m.stages) {
		parts = append(parts, m.renderHandoffOverlay(m.stages[m.overlayStageIndex]))
	}

	if m.runInProgress {
		timer := fmt.Sprintf("%.1fs", time.Since(m.requestStartTime).Seconds())
		parts = append(parts, fmt.Sprintf("%s Running pipeline... %s", m.spinner.View(), timer))
	} else {
		parts = append(parts, m.textArea.View())
	}

	help := "Enter send  Ctrl+←/→ focus  Ctrl+Enter expand  Ctrl+S cycle  Ctrl+O overlay  Ctrl+P multimodel  Ctrl+E export  Ctrl+Q quit"
	parts = append(parts, lipgloss.NewStyle().Faint(true).Render(help))

	return lipgloss.NewStyle().Margin(1, 2).Render(strings.Join(parts, "\n\n"))
}

func (m *pipelineModel) expandedView() string {
	if m.expandedIndex < 0 || m.expandedIndex >= len(m.stages) {
		return "No stage selected"
	}

	stage := m.stages[m.expandedIndex]
	var builder strings.Builder

	header := fmt.Sprintf("Stage %d — %s • %s", stage.index+1, stage.host.Name, stage.selectedModel)
	builder.WriteString(stageTitleStyle.Render(header) + "\n")
	builder.WriteString(stageStatusStyles[stage.status].Render(stage.statusMessage) + "\n\n")

	switch stage.view {
	case pipelineStageViewOutput:
		if stage.finalOutput == "" {
			builder.WriteString("(no output yet)")
		} else {
			builder.WriteString(stage.finalOutput)
		}
	case pipelineStageViewStats:
		builder.WriteString(m.renderStageStats(stage))
	case pipelineStageViewHandoff:
		builder.WriteString(stage.handoff.payload)
	}

	builder.WriteString("\n\n")
	builder.WriteString(lipgloss.NewStyle().Faint(true).Render("Esc close  Ctrl+←/→ stage  Ctrl+S cycle view"))

	return lipgloss.NewStyle().Margin(1, 2).Render(builder.String())
}

func (m *pipelineModel) renderProgressLine() string {
	stageNames := make([]string, len(m.stages))
	for i, stage := range m.stages {
		if stage.hasAssignment {
			stageNames[i] = stage.host.Name
		} else {
			stageNames[i] = "—"
		}
	}

	pipelinePath := strings.Join(stageNames, " → ")

	currentStage, totalAssigned, stageLabel := m.currentStageProgress()
	stageStatus := "Stage 0/0 Idle"
	if totalAssigned > 0 {
		stageStatus = fmt.Sprintf("Stage %d/%d %s", currentStage, totalAssigned, stageLabel)
	}

	speed := "t/s: --"
	if avg := m.averageTokensPerSecond(); avg > 0 {
		speed = fmt.Sprintf("%.1f t/s", avg)
	}

	ttft := "TTFT: --"
	if stage := m.currentRunningStage(); stage != nil {
		if !stage.firstToken.IsZero() {
			ttft = fmt.Sprintf("TTFT: %.1fs", stage.firstToken.Sub(stage.startedAt).Seconds())
		} else {
			ttft = fmt.Sprintf("TTFT: %.1fs", time.Since(stage.startedAt).Seconds())
		}
	}

	jsonMode := "jsonMode: off"
	if m.config.JSONMode {
		jsonMode = "jsonMode: on"
	}
	mcpIndicator := formatMCPIndicator(m.mcpStatus)

	return fmt.Sprintf("Pipeline: %s | %s | %s | %s | %s | %s", pipelinePath, stageStatus, speed, ttft, jsonMode, mcpIndicator)
}

func (m *pipelineModel) renderStageColumns(targetHeight int) string {
	colWidth := util.Max(30, (m.width-8)/pipelineStageCount)
	var columns []string

	for i, stage := range m.stages {
		column := m.renderStageColumn(stage, colWidth, targetHeight)
		wrapper := normalColumn.Width(colWidth)
		if i == m.focusIndex {
			wrapper = focusedColumn.Width(colWidth)
		}
		columns = append(columns, wrapper.Render(column))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}

func (m *pipelineModel) renderStageColumn(stage pipelineStage, colWidth int, targetHeight int) string {
	var headerLines []string
	title := fmt.Sprintf("Stage %d", stage.index+1)
	headerLines = append(headerLines, stageTitleStyle.Render(title))

	if stage.hasAssignment {
		badge := fmt.Sprintf("%s • %s", stage.host.Name, stage.selectedModel)
		headerLines = append(headerLines, stageBadgeStyle.Render(badge))
	} else {
		headerLines = append(headerLines, stageBadgeStyle.Render("(unassigned)"))
	}

	statusChip := stageStatusStyles[stage.status].Render(stage.statusMessage)
	if stage.cacheHit {
		statusChip = stageCacheStyle.Render("⟳ ") + statusChip
	}

	header := lipgloss.JoinVertical(lipgloss.Left, headerLines...)
	header = lipgloss.JoinHorizontal(lipgloss.Top, header, lipgloss.NewStyle().Width(colWidth-lipgloss.Width(header)).Align(lipgloss.Right).Render(statusChip))

	// Render body content constrained to a target number of lines
	body := m.renderStageBody(stage, colWidth)
	headerLineCount := strings.Count(header, "\n") + 1

	bodyLines := strings.Split(body, "\n")
	maxBodyLines := targetHeight - headerLineCount - 0 // 0 because we add exactly one newline between header and body
	if maxBodyLines < 0 {
		maxBodyLines = 0
	}
	if len(bodyLines) > maxBodyLines {
		bodyLines = bodyLines[:maxBodyLines]
	} else if len(bodyLines) < maxBodyLines {
		// pad with blank lines to fill remaining space
		pad := make([]string, maxBodyLines-len(bodyLines))
		bodyLines = append(bodyLines, pad...)
	}
	body = strings.Join(bodyLines, "\n")

	return header + "\n" + body
}

func (m *pipelineModel) renderStageBody(stage pipelineStage, colWidth int) string {
	switch stage.view {
	case pipelineStageViewStats:
		return m.renderStageStats(stage)
	case pipelineStageViewHandoff:
		if stage.handoff.preview == "" {
			return stageBadgeStyle.Render("Handoff pending")
		}
		preview := stage.handoff.preview
		if stage.handoff.truncated {
			preview += "\n" + stageBadgeStyle.Render(stage.handoff.truncationSummary)
		}
		return preview + "\n" + stageBadgeStyle.Render("Ctrl+O for details")
	default:
		if stage.finalOutput != "" {
			// Wrap long lines so content stays within the column
			return util.WrapToWidth(stage.finalOutput, colWidth-4)
		}
		if stage.status == pipelineStageStatusWaiting {
			if stage.index > 0 {
				return stageBadgeStyle.Render(fmt.Sprintf("Waiting for input from Stage %d", stage.index))
			}
			return stageBadgeStyle.Render("Waiting for prompt")
		}
		if stage.status == pipelineStageStatusRunning {
			return stageBadgeStyle.Render("Streaming response...")
		}
		return stageBadgeStyle.Render("No output yet")
	}
}

func (m *pipelineModel) renderStageStats(stage pipelineStage) string {
	if stage.stats.Model == "" {
		return stageBadgeStyle.Render("No stats yet")
	}

	total := float64(stage.stats.TotalDuration) / 1e9
	load := float64(stage.stats.LoadDuration) / 1e9
	prompt := float64(stage.stats.PromptEvalDuration) / 1e9
	eval := float64(stage.stats.EvalDuration) / 1e9
	tokensPerSecond := m.tokensPerSecond(stage.stats)

	stats := []string{
		fmt.Sprintf("Model: %s", stage.stats.Model),
		fmt.Sprintf("Total: %.2fs", total),
		fmt.Sprintf("Load: %.2fs", load),
		fmt.Sprintf("Prompt Eval: %.2fs (%d tokens)", prompt, stage.stats.PromptEvalCount),
		fmt.Sprintf("Eval: %.2fs (%d tokens)", eval, stage.stats.EvalCount),
		fmt.Sprintf("Tokens/s: %.2f", tokensPerSecond),
	}

	return strings.Join(stats, "\n")
}

func (m *pipelineModel) renderHandoffOverlay(stage pipelineStage) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Stage %d handoff", stage.index+1) + "\n")
	mode := "raw"
	switch stage.handoff.mode {
	case pipelineHandoffSelector:
		mode = "selector"
	case pipelineHandoffTemplate:
		mode = "template"
	}
	builder.WriteString(fmt.Sprintf("Mode: %s\n", mode))
	builder.WriteString(fmt.Sprintf("Tokens: %d\n", stage.handoff.tokenCount))
	if stage.handoff.truncated {
		builder.WriteString(stage.handoff.truncationSummary + "\n")
	}
	if len(stage.handoff.redactions) > 0 {
		builder.WriteString("Redactions:\n")
		for _, r := range stage.handoff.redactions {
			builder.WriteString("  • " + r + "\n")
		}
	}
	builder.WriteString("\nPreview:\n" + stage.handoff.preview)

	return overlayStyle.Render(builder.String())
}

func (m *pipelineModel) moveFocus(delta int) {
	newIndex := m.focusIndex
	for attempt := 0; attempt < pipelineStageCount; attempt++ {
		newIndex = (newIndex + delta + pipelineStageCount) % pipelineStageCount
		if newIndex >= 0 && newIndex < len(m.stages) {
			break
		}
	}
	m.focusIndex = newIndex
}

func (m *pipelineModel) startPipelineRun(input string) tea.Cmd {
	m.runInProgress = true
	m.viewState = pipelineViewRunning
	m.requestStartTime = time.Now()
	m.runStarted = time.Now()
	m.runCompleted = time.Time{}
	m.exportRecords = nil
	m.textArea.Reset()
	m.textArea.Blur()
	m.statusBanner = ""

	for i := range m.stages {
		stage := &m.stages[i]
		stage.outputBuffer.Reset()
		stage.finalOutput = ""
		stage.stats = LLMResponseMeta{}
		stage.cacheHit = false
		stage.firstToken = time.Time{}
		stage.completedAt = time.Time{}
		stage.startedAt = time.Time{}
		stage.handoff = pipelineHandoff{mode: pipelineHandoffRaw}
		if stage.hasAssignment {
			stage.status = pipelineStageStatusWaiting
			stage.statusMessage = "Waiting"
			stage.history = []chatMessage{{Role: "user", Content: input}}
		} else {
			stage.status = pipelineStageStatusSkipped
			stage.statusMessage = "Skipped"
			stage.history = nil
		}
	}

	m.stageInputs = [pipelineStageCount]string{}

	first := m.firstAssignedStage()
	if first == -1 {
		m.runInProgress = false
		m.viewState = pipelineViewReady
		m.statusBanner = "No stages assigned"
		m.textArea.Focus()
		return nil
	}

	if first < len(m.stageInputs) {
		m.stageInputs[first] = input
	}
	m.focusIndex = first

	return tea.Batch(m.spinner.Tick, m.queueStage(first))
}

func (m *pipelineModel) queueStage(index int) tea.Cmd {
	if index < 0 || index >= len(m.stages) {
		return nil
	}

	payload := ""
	if index < len(m.stageInputs) {
		payload = m.stageInputs[index]
	}

	stage := &m.stages[index]
	if !stage.hasAssignment {
		return m.advanceToNextStage(index, payload)
	}

	cacheKey := makeCacheKey(index, stage.host.URL, stage.selectedModel, payload)
	if entry, ok := m.memoCache[cacheKey]; ok {
		stage.cacheHit = true
		return func() tea.Msg { return pipelineStageCacheHitMsg{Stage: index, Entry: entry} }
	}

	stage.status = pipelineStageStatusRunning
	stage.statusMessage = "Running"
	stage.startedAt = time.Now()
	stage.cacheHit = false
	stage.outputBuffer.Reset()

	messages := append([]chatMessage(nil), stage.history...)
	if payload != "" {
		if len(messages) == 0 || messages[len(messages)-1].Role != "user" || messages[len(messages)-1].Content != payload {
			messages = append(messages, chatMessage{Role: "user", Content: payload})
		}
	}

	return pipelineStreamStageCmd(m.ctx, m.program, m.provider, index, stage.host, stage.selectedModel, messages, stage.systemPrompt, stage.parameters, payload, m.config.JSONMode, m.requestTimeout)
}

func (m *pipelineModel) advanceToNextStage(current int, payload string) tea.Cmd {
	next := m.findNextAssignedStage(current + 1)
	if next == -1 {
		m.runInProgress = false
		m.viewState = pipelineViewReady
		if m.runCompleted.IsZero() {
			m.runCompleted = time.Now()
		}
		m.autoExport()
		m.textArea.Focus()
		return nil
	}
	if next < len(m.stageInputs) {
		m.stageInputs[next] = payload
	}
	m.focusIndex = next
	return m.queueStage(next)
}

func (m *pipelineModel) handleStageChunk(msg pipelineStageChunkMsg) {
	if msg.Stage < 0 || msg.Stage >= len(m.stages) {
		return
	}
	stage := &m.stages[msg.Stage]
	if stage.firstToken.IsZero() {
		stage.firstToken = time.Now()
	}
	stage.outputBuffer.WriteString(msg.Content)
}

func (m *pipelineModel) handleStageDone(msg pipelineStageDoneMsg) tea.Cmd {
	if msg.Stage < 0 || msg.Stage >= len(m.stages) {
		return nil
	}

	stage := &m.stages[msg.Stage]
	stage.finalOutput = stage.outputBuffer.String()
	stage.stats = msg.Meta
	stage.status = pipelineStageStatusDone
	stage.statusMessage = m.formatCompletionStatus(msg.Meta)
	stage.completedAt = time.Now()

	stage.history = append(stage.history, chatMessage{Role: "assistant", Content: stage.finalOutput})

	if !m.prepareHandoff(stage) {
		stage.status = pipelineStageStatusError
		stage.statusMessage = "JSON validation failed"
		m.runInProgress = false
		m.viewState = pipelineViewReady
		if m.runCompleted.IsZero() {
			m.runCompleted = time.Now()
		}
		m.textArea.Focus()
		return nil
	}

	inbound := ""
	if msg.Stage < len(m.stageInputs) {
		inbound = m.stageInputs[msg.Stage]
	}

	cacheKey := makeCacheKey(msg.Stage, stage.host.URL, stage.selectedModel, inbound)
	m.memoCache[cacheKey] = pipelineCacheEntry{output: stage.finalOutput, meta: msg.Meta, handoff: stage.handoff, timestamp: time.Now()}

	m.exportRecords = append(m.exportRecords, m.buildExportRecord(msg.Stage, stage))

	return m.advanceToNextStage(msg.Stage, stage.handoff.payload)
}

func (m *pipelineModel) handleStageError(msg pipelineStageErrorMsg) {
	if msg.Stage < 0 || msg.Stage >= len(m.stages) {
		return
	}
	stage := &m.stages[msg.Stage]
	stage.status = pipelineStageStatusError
	stage.statusMessage = "Error"
	m.statusBanner = fmt.Sprintf("Stage %d error: %v", stage.index+1, msg.Err)
	m.runInProgress = false
	m.viewState = pipelineViewReady
	if m.runCompleted.IsZero() {
		m.runCompleted = time.Now()
	}
	m.textArea.Focus()
}

func (m *pipelineModel) handleStageCacheHit(msg pipelineStageCacheHitMsg) tea.Cmd {
	if msg.Stage < 0 || msg.Stage >= len(m.stages) {
		return nil
	}
	stage := &m.stages[msg.Stage]
	stage.finalOutput = msg.Entry.output
	stage.stats = msg.Entry.meta
	stage.handoff = msg.Entry.handoff
	stage.status = pipelineStageStatusDone
	stage.statusMessage = "Cached"
	stage.cacheHit = true
	stage.completedAt = time.Now()
	stage.history = append(stage.history, chatMessage{Role: "assistant", Content: stage.finalOutput})

	m.exportRecords = append(m.exportRecords, m.buildExportRecord(msg.Stage, stage))

	return m.advanceToNextStage(msg.Stage, stage.handoff.payload)
}

func (m *pipelineModel) prepareHandoff(stage *pipelineStage) bool {
	payload := strings.TrimSpace(stage.finalOutput)
	if payload == "" {
		stage.handoff = pipelineHandoff{mode: pipelineHandoffRaw, payload: "", preview: "(empty)", tokenCount: 0}
		return true
	}

	if m.config.JSONMode {
		if !json.Valid([]byte(payload)) {
			repaired, ok := attemptJSONRepair(payload)
			if ok {
				payload = repaired
			} else {
				return false
			}
		}
	}

	tokens := len(strings.Fields(payload))
	truncated := false
	if tokens > pipelineMaxHandoffTokens {
		truncated = true
		fields := strings.Fields(payload)
		payload = strings.Join(fields[len(fields)-pipelineMaxHandoffTokens:], " ")
	}

	preview := util.TruncateRunes(payload, pipelinePreviewRunes)
	summary := ""
	if truncated {
		summary = fmt.Sprintf("Truncated (tail, %d tokens)", pipelineMaxHandoffTokens)
	}

	stage.handoff = pipelineHandoff{
		mode:              pipelineHandoffRaw,
		payload:           payload,
		preview:           preview,
		truncated:         truncated,
		truncationSummary: summary,
		tokenCount:        util.Min(tokens, pipelineMaxHandoffTokens),
	}
	return true
}

func (m *pipelineModel) anyStageAssigned() bool {
	for _, stage := range m.stages {
		if stage.hasAssignment {
			return true
		}
	}
	return false
}

func (m *pipelineModel) preflightAssignments() error {
	for i, stage := range m.stages {
		if stage.hasAssignment {
			if stage.host.URL == "" {
				return fmt.Errorf("Stage %d: host missing URL", i+1)
			}
			if stage.selectedModel == "" {
				return fmt.Errorf("Stage %d: model not selected", i+1)
			}
		}
	}
	return nil
}

func (m *pipelineModel) firstAssignedStage() int {
	for i, stage := range m.stages {
		if stage.hasAssignment {
			return i
		}
	}
	return -1
}

func (m *pipelineModel) findNextAssignedStage(start int) int {
	for i := start; i < len(m.stages); i++ {
		if m.stages[i].hasAssignment {
			return i
		}
	}
	return -1
}

func (m *pipelineModel) currentStageProgress() (int, int, string) {
	totalAssigned := 0
	completed := 0
	for _, stage := range m.stages {
		if stage.hasAssignment {
			totalAssigned++
			if stage.status == pipelineStageStatusDone {
				completed++
			}
		}
	}
	if totalAssigned == 0 {
		return 0, 0, "Idle"
	}
	if running := m.currentRunningStage(); running != nil {
		return running.index + 1, totalAssigned, "Running"
	}
	if completed == totalAssigned {
		return totalAssigned, totalAssigned, "Done"
	}
	if m.runInProgress {
		return util.Min(completed+1, totalAssigned), totalAssigned, "Waiting"
	}
	if completed > 0 {
		return completed, totalAssigned, "Idle"
	}
	return 1, totalAssigned, "Idle"
}

func (m *pipelineModel) averageTokensPerSecond() float64 {
	totalTokens := 0
	totalDuration := 0.0
	for _, stage := range m.stages {
		if stage.status == pipelineStageStatusDone && stage.stats.EvalDuration > 0 {
			totalTokens += stage.stats.EvalCount
			totalDuration += float64(stage.stats.EvalDuration) / 1e9
		}
	}
	if totalTokens == 0 || totalDuration == 0 {
		return 0
	}
	return float64(totalTokens) / totalDuration
}

func (m *pipelineModel) currentRunningStage() *pipelineStage {
	for i := range m.stages {
		if m.stages[i].status == pipelineStageStatusRunning {
			return &m.stages[i]
		}
	}
	return nil
}

func (m *pipelineModel) formatCompletionStatus(meta LLMResponseMeta) string {
	if meta.EvalDuration == 0 {
		return "Done"
	}
	tps := m.tokensPerSecond(meta)
	total := float64(meta.TotalDuration) / 1e9
	return fmt.Sprintf("Done • %.1fs • %.1f t/s", total, tps)
}

func (m *pipelineModel) tokensPerSecond(meta LLMResponseMeta) float64 {
	if meta.EvalDuration == 0 {
		return 0
	}
	return float64(meta.EvalCount) / (float64(meta.EvalDuration) / 1e9)
}

func (m *pipelineModel) buildExportRecord(idx int, stage *pipelineStage) pipelineExportRecord {
	hash := fnv.New64a()
	hash.Write([]byte(stage.systemPrompt))
	promptHash := fmt.Sprintf("%x", hash.Sum64())

	outputHash := fnv.New64a()
	outputHash.Write([]byte(stage.finalOutput))

	timings := exportTimings{
		TotalSeconds:      float64(stage.stats.TotalDuration) / 1e9,
		LoadSeconds:       float64(stage.stats.LoadDuration) / 1e9,
		PromptEvalSeconds: float64(stage.stats.PromptEvalDuration) / 1e9,
		EvalSeconds:       float64(stage.stats.EvalDuration) / 1e9,
	}
	if !stage.firstToken.IsZero() && !stage.startedAt.IsZero() {
		timings.TimeToFirstToken = stage.firstToken.Sub(stage.startedAt).Seconds()
	}

	return pipelineExportRecord{
		Stage:             idx + 1,
		Host:              stage.host.Name,
		Model:             stage.selectedModel,
		Parameters:        stage.parameters,
		SystemPromptHash:  promptHash,
		Timings:           timings,
		Tokens:            exportTokens{Prompt: stage.stats.PromptEvalCount, Eval: stage.stats.EvalCount},
		OutputHash:        fmt.Sprintf("%x", outputHash.Sum64()),
		HandoffPayload:    stage.handoff.payload,
		CacheHit:          stage.cacheHit,
		TruncationSummary: stage.handoff.truncationSummary,
	}
}

// exportPipelineJSON writes the latest run data to pipeline.json in the current directory.

func (m *pipelineModel) autoExport() {
	if len(m.exportRecords) == 0 {
		return
	}
	var errs []string
	if path := strings.TrimSpace(m.exportPath); path != "" {
		if err := m.exportPipelineJSON(path); err != nil {
			errs = append(errs, fmt.Sprintf("JSON export failed: %v", err))
		}
	}
	if path := strings.TrimSpace(m.exportMarkdownPath); path != "" {
		if err := m.exportPipelineMarkdown(path); err != nil {
			errs = append(errs, fmt.Sprintf("Markdown export failed: %v", err))
		}
	}
	if len(errs) > 0 {
		m.statusBanner = strings.Join(errs, " | ")
	}
}

func (m *pipelineModel) exportPipelineJSON(path string) error {
	if len(m.exportRecords) == 0 {
		return fmt.Errorf("no pipeline run to export")
	}
	export := struct {
		RunStarted   time.Time              `json:"runStarted"`
		RunCompleted time.Time              `json:"runCompleted"`
		JSONMode     bool                   `json:"jsonMode"`
		Stages       []pipelineExportRecord `json:"stages"`
	}{
		RunStarted: m.runStarted,
		RunCompleted: func() time.Time {
			if m.runCompleted.IsZero() {
				return time.Now()
			}
			return m.runCompleted
		}(),
		JSONMode: m.config.JSONMode,
		Stages:   m.exportRecords,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}

	return util.WriteFile(path, data)
}

func (m *pipelineModel) exportPipelineMarkdown(path string) error {
	if len(m.exportRecords) == 0 {
		return fmt.Errorf("no pipeline run to export")
	}
	builder := &strings.Builder{}
	runCompleted := m.runCompleted
	if runCompleted.IsZero() {
		runCompleted = time.Now()
	}
	builder.WriteString("# Pipeline Run\n\n")
	builder.WriteString(fmt.Sprintf("- Run started: %s\n", m.runStarted.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("- Run completed: %s\n", runCompleted.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("- JSON mode: %t\n\n", m.config.JSONMode))
	for _, rec := range m.exportRecords {
		builder.WriteString(fmt.Sprintf("## Stage %d — %s (%s)\n\n", rec.Stage, rec.Host, rec.Model))
		builder.WriteString(fmt.Sprintf("- Cache hit: %t\n", rec.CacheHit))
		builder.WriteString(fmt.Sprintf("- Prompt tokens: %d\n", rec.Tokens.Prompt))
		builder.WriteString(fmt.Sprintf("- Eval tokens: %d\n", rec.Tokens.Eval))
		builder.WriteString(fmt.Sprintf("- Total seconds: %.2f\n", rec.Timings.TotalSeconds))
		builder.WriteString(fmt.Sprintf("- Load seconds: %.2f\n", rec.Timings.LoadSeconds))
		builder.WriteString(fmt.Sprintf("- Prompt eval seconds: %.2f\n", rec.Timings.PromptEvalSeconds))
		builder.WriteString(fmt.Sprintf("- Eval seconds: %.2f\n", rec.Timings.EvalSeconds))
		builder.WriteString(fmt.Sprintf("- Time to first token: %.2f\n", rec.Timings.TimeToFirstToken))
		if rec.TruncationSummary != "" {
			builder.WriteString(fmt.Sprintf("- Handoff: %s\n", rec.TruncationSummary))
		}
		builder.WriteString("\n```text\n")
		builder.WriteString(rec.HandoffPayload)
		builder.WriteString("\n```\n\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

// StartPipelineGUI initializes the pipeline Bubble Tea program and blocks until exit.

func StartPipelineGUI(ctx context.Context, cfg *Config, cancel context.CancelFunc) error {
	provider, err := providerfactory.NewChatProvider(cfg)
	if err != nil {
		// This can only happen if MCP mode is enabled and fails to start.
		// Fallback to a direct Ollama provider.
		provider = ollama.New(cfg)
	}

	m := initialPipelineModel(ctx, cfg, provider)
	m.client = &http.Client{
		Transport: &http.Transport{ForceAttemptHTTP2: false},
		Timeout:   m.requestTimeout,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	_, runErr := p.Run()

	if m.switchToMultimodel {
		if provider == nil {
			provider, err = providerfactory.NewChatProvider(cfg)
			if err != nil {
				if cfg.MCPMode {
					logging.LogEvent("MCP provider unavailable: %v — falling back to direct Ollama access", err)
					provider = ollama.New(cfg)
				} else {
					return err
				}
			}
		}
		multiErr := StartMultimodelGUI(m.ctx, cfg, provider, cancel)
		if provider != nil {
			if cerr := provider.Close(); cerr != nil && multiErr == nil {
				multiErr = cerr
			}
		}
		return multiErr
	}

	if provider != nil {
		if cerr := provider.Close(); cerr != nil && runErr == nil {
			runErr = cerr
		}
	}

	return runErr
}

// pipelineStreamStageCmd streams a stage response and emits updates to the Bubble Tea program.

func pipelineStreamStageCmd(pctx context.Context, p *tea.Program, chatProvider providers.ChatProvider, stageIndex int, host Host, modelName string, history []chatMessage, systemPrompt string, parameters Parameters, payload string, jsonMode bool, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(pctx, timeout)
		request := providers.StreamRequest{
			Host:         host,
			Model:        modelName,
			History:      history,
			SystemPrompt: systemPrompt,
			Parameters:   parameters,
			JSONMode:     jsonMode,
		}
		go func() {
			defer cancel()
			err := chatProvider.Stream(ctx, request, providers.StreamCallbacks{
				OnChunk: func(msg providers.ChatMessage) error {
					if msg.Content != "" {
						p.Send(pipelineStageChunkMsg{Stage: stageIndex, Content: msg.Content})
					}
					return nil
				},
				OnComplete: func(meta providers.StreamMetadata) error {
					if meta.Model == "" {
						meta.Model = modelName
					}
					p.Send(pipelineStageDoneMsg{Stage: stageIndex, Meta: meta})
					return nil
				},
			})
			if err != nil {
				p.Send(pipelineStageErrorMsg{Stage: stageIndex, Err: err})
			}
		}()
		return nil
	}
}

func attemptJSONRepair(output string) (string, bool) {
	trimmed := strings.TrimSpace(output)
	if json.Valid([]byte(trimmed)) {
		return trimmed, true
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		candidate := trimmed[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, true
		}
	}

	start = strings.Index(trimmed, "[")
	end = strings.LastIndex(trimmed, "]")
	if start >= 0 && end > start {
		candidate := trimmed[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, true
		}
	}

	return output, false
}

func makeCacheKey(stage int, hostURL, modelName, payload string) string {
	hash := fnv.New64a()
	hash.Write([]byte(fmt.Sprintf("%d|%s|%s|", stage, hostURL, modelName)))
	hash.Write([]byte(payload))
	return fmt.Sprintf("%x", hash.Sum64())
}
