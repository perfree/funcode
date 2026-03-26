package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/perfree/funcode/internal/agent"
	"github.com/perfree/funcode/internal/skill"
	"github.com/perfree/funcode/internal/version"
	"github.com/perfree/funcode/pkg/types"
)

// programRef is a shared pointer so goroutines can Send messages
// even after bubbletea copies the Model value.
type programRef struct {
	p *tea.Program
}

// ProgramRef is the exported version for main.go to set
type ProgramRef struct {
	P *tea.Program
}

// ── Messages ────────────────────────────────────────────────

type StreamChunkMsg struct{ Event types.StreamEvent }
type AgentDoneMsg struct {
	Response string
	Error    error
	RoleName string // which role finished (for @all)
	Elapsed  time.Duration
}
type ToolCallMsg struct{ Call types.ToolCall }
type ToolResultMsg struct {
	Call   types.ToolCall
	Result types.ToolResult
}

// AllAgentDoneMsg signals one agent in an @all broadcast finished
type AllAgentDoneMsg struct {
	RoleName string
	Response string
	Error    error
	Elapsed  time.Duration
	Usage    types.Usage
}

// AllBroadcastCompleteMsg signals all @all agents finished
type AllBroadcastCompleteMsg struct{}

// DelegateActivityMsg signals a sub-agent activity during delegation
type DelegateActivityMsg struct {
	ParentCallID string
	RoleName     string
	Call         types.ToolCall
	Result       *types.ToolResult // nil=tool starting, non-nil=tool completed
}

type DelegateStreamMsg struct {
	ParentCallID string
	RoleName     string
	Event        types.StreamEvent
}

type AgentActivityMsg struct {
	ParentCallID string
	WorkerLabel  string
	Call         types.ToolCall
	Result       *types.ToolResult // nil=tool starting, non-nil=tool completed
}

type AgentStreamMsg struct {
	ParentCallID string
	WorkerLabel  string
	Event        types.StreamEvent
}

// ApprovalRequestMsg asks user to approve a tool call
type ApprovalRequestMsg struct {
	ToolName string
	Params   string
	ReplyCh  chan int // 0=allow, 1=always, 2=deny
}

type CommandInfo struct {
	Name        string
	Description string
}

type printTranscriptMsg struct {
	content string
}

type pendingSubmitMsg struct {
	seq int
}

type LiveToolBlock struct {
	CallID     string
	Label      string
	Lines      []string
	StreamText string
	StartedAt  time.Time
}

const mainUsageKey = "__main__"

// ── Model ───────────────────────────────────────────────────

type Model struct {
	input   InputModel
	spinner SpinnerModel
	theme   *Theme

	messages    []ChatMessage
	streamBuf   string
	streaming   bool
	width       int
	height      int
	showWelcome bool
	scrollBack  int
	autoScroll  bool

	orchestrator       *agent.Orchestrator
	agentCancel        context.CancelFunc
	currentRole        string
	currentRoleDisplay string

	// @ mention popup
	showMention   bool
	mentionItems  []agent.AgentInfo
	mentionCursor int

	// / command popup
	showCommand   bool
	commandItems  []CommandInfo
	commandCursor int

	// @all tracking
	allPending int

	// Approval prompt
	showApproval    bool
	approvalTool    string
	approvalParams  string
	approvalCursor  int // 0=allow once, 1=always allow, 2=deny
	approvalReplyCh chan int

	pRef *programRef

	totalTokens                int
	totalMsgs                  int
	modelName                  string
	cwd                        string
	markdownRender             bool
	expandTools                bool // toggle to expand all tool results
	inputHistory               []string
	historyCursor              int
	historyDraft               string
	turnAssistantHeaderPrinted bool
	lastTurnNeedsSeparator     bool // separator deferred until next turn
	currentRunStartedAt        time.Time

	skillManager        *skill.Manager
	activeSkill         *skill.Skill
	pendingSubmit       bool
	pendingSubmitSeq    int
	toolStartTimes      map[string]time.Time
	activeUsages        map[string]types.Usage
	turnEstimatedTokens int
	turnHasRealUsage    bool
	liveToolBlocks      []LiveToolBlock
}

func New(orchestrator *agent.Orchestrator, modelName string, markdownRender bool, skillManager *skill.Manager) Model {
	cwd, _ := os.Getwd()
	theme := DefaultTheme()
	input := NewInput("  > ")
	input.MentionStyle = theme.MentionHighlight
	return Model{
		input:              input,
		spinner:            NewSpinner(),
		theme:              theme,
		orchestrator:       orchestrator,
		modelName:          modelName,
		currentRole:        "developer",
		currentRoleDisplay: "Developer",
		showWelcome:        true,
		autoScroll:         true,
		pRef:               &programRef{},
		cwd:                cwd,
		markdownRender:     markdownRender,
		historyCursor:      -1,
		skillManager:       skillManager,
		toolStartTimes:     map[string]time.Time{},
		activeUsages:       map[string]types.Usage{},
	}
}

func (m Model) Init() tea.Cmd { return nil }

// ── Update ──────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 6)
		return m, nil

	case tea.KeyMsg:
		if m.pendingSubmit && !m.streaming && !m.showApproval && !m.showMention && !m.showCommand {
			m.pendingSubmit = false
			m.input.InsertString("\n")
		}

		if msg.Type == tea.KeyRunes {
			if looksLikePaste(msg) {
				// no-op: paste-like detection is kept for future heuristics
			}
		}

		// ── Approval prompt key handling (highest priority) ──
		if m.showApproval {
			switch msg.Type {
			case tea.KeyLeft:
				if m.approvalCursor > 0 {
					m.approvalCursor--
				}
				return m, nil
			case tea.KeyRight:
				if m.approvalCursor < 2 {
					m.approvalCursor++
				}
				return m, nil
			case tea.KeyEnter:
				m.showApproval = false
				if m.approvalReplyCh != nil {
					m.approvalReplyCh <- m.approvalCursor
				}
				cmd := m.spinner.Start("Executing...")
				return m, cmd
			case tea.KeyEscape:
				// Escape = deny
				m.showApproval = false
				if m.approvalReplyCh != nil {
					m.approvalReplyCh <- 2
				}
				cmd := m.spinner.Start("Denied, thinking...")
				return m, cmd
			}
			// 快捷键: y=allow, a=always, n=deny
			if msg.Type == tea.KeyRunes {
				switch string(msg.Runes) {
				case "y", "Y":
					m.showApproval = false
					if m.approvalReplyCh != nil {
						m.approvalReplyCh <- 0
					}
					cmd := m.spinner.Start("Executing...")
					return m, cmd
				case "a", "A":
					m.showApproval = false
					if m.approvalReplyCh != nil {
						m.approvalReplyCh <- 1
					}
					cmd := m.spinner.Start("Executing...")
					return m, cmd
				case "n", "N":
					m.showApproval = false
					if m.approvalReplyCh != nil {
						m.approvalReplyCh <- 2
					}
					cmd := m.spinner.Start("Denied, thinking...")
					return m, cmd
				}
			}
			return m, nil
		}

		// Ctrl+B to toggle expand/collapse all tool results
		if msg.String() == "ctrl+b" {
			m.expandTools = !m.expandTools
			return m, nil
		}

		switch msg.Type {
		case tea.KeyPgUp:
			m.scrollBack += m.contentHeight() / 2
			m.autoScroll = false
			return m, nil
		case tea.KeyPgDown:
			m.scrollBack -= m.contentHeight() / 2
			if m.scrollBack <= 0 {
				m.scrollBack = 0
				m.autoScroll = true
			}
			return m, nil
		}
		if msg.String() == "shift+up" {
			m.scrollBack += 3
			m.autoScroll = false
			return m, nil
		}
		if msg.String() == "shift+down" {
			m.scrollBack -= 3
			if m.scrollBack <= 0 {
				m.scrollBack = 0
				m.autoScroll = true
			}
			return m, nil
		}

		// ── Mention popup key handling ──
		if m.showMention {
			switch msg.Type {
			case tea.KeyEscape:
				m.showMention = false
				return m, nil
			case tea.KeyUp:
				if m.mentionCursor > 0 {
					m.mentionCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.mentionCursor < len(m.mentionItems)-1 {
					m.mentionCursor++
				}
				return m, nil
			case tea.KeyTab, tea.KeyEnter:
				if m.mentionCursor < len(m.mentionItems) {
					selected := m.mentionItems[m.mentionCursor]
					val := m.input.Value()
					atIdx := strings.LastIndex(val, "@")
					prefix := ""
					if atIdx > 0 {
						prefix = val[:atIdx]
					}
					m.input.SetValue(prefix + "@" + selected.Name + " ")
					m.showMention = false
				}
				return m, nil
			default:
				// Continue typing to filter — update input, then re-filter
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
				m.filterMention()
				return m, tea.Batch(cmds...)
			}
		}

		// ── Slash command popup key handling ──
		if m.showCommand {
			switch msg.Type {
			case tea.KeyEscape:
				m.showCommand = false
				return m, nil
			case tea.KeyUp:
				if m.commandCursor > 0 {
					m.commandCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.commandCursor < len(m.commandItems)-1 {
					m.commandCursor++
				}
				return m, nil
			case tea.KeyTab, tea.KeyEnter:
				if m.commandCursor < len(m.commandItems) {
					m.input.SetValue(m.commandItems[m.commandCursor].Name)
					m.showCommand = false
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
				m.filterCommand()
				return m, tea.Batch(cmds...)
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.streaming && m.agentCancel != nil {
				m.agentCancel()
				m.streaming = false
				m.spinner.Stop()
				m.streamBuf = ""
				m.input.Focus()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEscape:
			m.showMention = false
			m.showCommand = false
			return m, nil

		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			m.pendingSubmit = true
			m.pendingSubmitSeq++
			return m, delayedSubmitCmd(m.pendingSubmitSeq)

		default:
			if !m.streaming {
				if msg.Type == tea.KeyUp {
					m.historyUp()
					return m, nil
				}
				if msg.Type == tea.KeyDown {
					m.historyDown()
					return m, nil
				}

				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)

				// Detect @ typing to show mention popup
				val := m.input.Value()
				if strings.HasSuffix(val, "@") || (strings.Contains(val, "@") && !strings.Contains(val, " ")) {
					m.openMention()
				} else if strings.HasPrefix(val, "/") {
					m.openCommand()
				} else {
					m.showMention = false
					m.showCommand = false
				}
			}
		}

	case ApprovalRequestMsg:
		m.showApproval = true
		m.approvalTool = msg.ToolName
		m.approvalParams = msg.Params
		m.approvalCursor = 0
		m.approvalReplyCh = msg.ReplyCh
		m.spinner.Stop()
		return m, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollBack += 3
			m.autoScroll = false
		case tea.MouseButtonWheelDown:
			m.scrollBack -= 3
			if m.scrollBack <= 0 {
				m.scrollBack = 0
				m.autoScroll = true
			}
		}
		return m, nil

	case StreamChunkMsg:
		if msg.Event.Type == types.EventTextDelta {
			m.streamBuf += msg.Event.Content
			m.addEstimatedTokens(msg.Event.Content)
			if m.autoScroll {
				m.scrollBack = 0
			}
		}
		m.handleUsageEvent(mainUsageKey, msg.Event)
		if msg.Event.Type == types.EventError && msg.Event.Error != "" {
			// Print retry/error info immediately to conversation
			errLine := m.theme.ToolErr.Render("  " + msg.Event.Error)
			// Ensure header is printed first
			if !m.turnAssistantHeaderPrinted {
				m.turnAssistantHeaderPrinted = true
				ts := time.Now().Format("15:04:05")
				name := m.currentRoleDisplay
				if name == "" {
					name = "Assistant"
				}
				header := renderMessageHeader(m.theme, ChatMessage{Role: types.RoleAssistant, RoleName: name}) + " " + m.theme.Dimmed.Render("["+ts+"]")
				return m, tea.Sequence(printTranscriptCmd(header), printTranscriptCmd(errLine))
			}
			return m, printTranscriptCmd(errLine)
		}
		return m, nil

	case ToolCallMsg:
		startedAt := m.rememberToolStart(msg.Call.ID)
		// Flush pending stream text to messages
		var flushedMsg *ChatMessage
		if m.streamBuf != "" {
			cm := ChatMessage{
				Role: types.RoleAssistant, RoleName: m.currentRoleDisplay, Text: m.streamBuf,
			}
			m.messages = append(m.messages, cm)
			m.streamBuf = ""
			flushedMsg = &cm
		}
		if isFileToolCall(msg.Call) {
			// File tools: just update spinner, summary printed on result
			detail := summarizeFileToolCall(msg.Call)
			return m, m.spinner.StartWithStartedAt(detail, m.currentRunStartedAt)
		}
		if isDelegateToolCall(msg.Call) {
			// Delegate tools: keep a dedicated live block per delegate call
			label := formatToolCallName(msg.Call)
			m.addLiveToolBlock(msg.Call.ID, label, startedAt)
			spinCmd := m.spinner.StartWithStartedAt(label+"...", m.currentRunStartedAt)
			if flushedMsg != nil {
				flushCmd := m.printAssistantTranscriptCmd(*flushedMsg)
				return m, tea.Batch(flushCmd, spinCmd)
			}
			return m, spinCmd
		}
		if isAgentToolCall(msg.Call) {
			label := formatToolCallName(msg.Call)
			m.addLiveToolBlock(msg.Call.ID, label, startedAt)
			spinCmd := m.spinner.StartWithStartedAt(label+"...", m.currentRunStartedAt)
			if flushedMsg != nil {
				flushCmd := m.printAssistantTranscriptCmd(*flushedMsg)
				return m, tea.Batch(flushCmd, spinCmd)
			}
			return m, spinCmd
		}
		label := formatToolCallName(msg.Call)
		toolMsg := ChatMessage{
			Role:     types.RoleAssistant,
			RoleName: m.currentRoleDisplay,
			ToolCalls: []ToolCallDisplay{
				{Name: label, Status: "running", Duration: formatElapsedSince(startedAt)},
			},
		}
		m.messages = append(m.messages, toolMsg)
		cmd := m.spinner.StartWithStartedAt(fmt.Sprintf("Running %s...", label), m.currentRunStartedAt)
		return m, tea.Batch(m.printAssistantTranscriptCmd(toolMsg), cmd)

	case ToolResultMsg:
		elapsed := m.finishTool(msg.Call.ID)
		duration := formatElapsed(elapsed)
		if isFileToolCall(msg.Call) {
			// File tools: print one-line summary inline in conversation
			summary := summarizeFileToolResult(m.theme, msg.Call, msg.Result, duration)
			summaryCmd := printTranscriptCmd(summary)
			// Ensure assistant header is printed before any file tool summary
			if !m.turnAssistantHeaderPrinted {
				m.turnAssistantHeaderPrinted = true
				ts := time.Now().Format("15:04:05")
				name := m.currentRoleDisplay
				if name == "" {
					name = "Assistant"
				}
				header := renderMessageHeader(m.theme, ChatMessage{Role: types.RoleAssistant, RoleName: name}) + " " + m.theme.Dimmed.Render("["+ts+"]")
				return m, tea.Batch(tea.Sequence(printTranscriptCmd(header), summaryCmd), m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt))
			}
			return m, tea.Batch(summaryCmd, m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt))
		}
		if isDelegateToolCall(msg.Call) {
			// Delegate tools: finalize the dedicated live block into a completed box
			block := m.removeLiveToolBlock(msg.Call.ID)
			label := formatToolCallName(msg.Call)
			status := "completed"
			resultText := msg.Result.Content
			if msg.Result.Error != "" {
				status = "error"
				resultText = msg.Result.Error
			}
			var combined strings.Builder
			for _, line := range block.Lines {
				combined.WriteString(line)
				combined.WriteString("\n")
			}
			streamText := strings.TrimSpace(block.StreamText)
			if len(block.Lines) > 0 && streamText != "" {
				combined.WriteString("────────────────────\n")
			}
			if streamText != "" {
				combined.WriteString(streamText)
			} else if resultText != "" {
				combined.WriteString(resultText)
			}
			toolMsg := ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: m.currentRoleDisplay,
				ToolCalls: []ToolCallDisplay{
					{Name: label, Status: status, Duration: duration, Result: strings.TrimSpace(combined.String())},
				},
			}
			m.messages = append(m.messages, toolMsg)
			cmd := m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt)
			return m, tea.Batch(m.printAssistantTranscriptCmd(toolMsg), cmd)
		}
		if isAgentToolCall(msg.Call) {
			block := m.removeLiveToolBlock(msg.Call.ID)
			label := formatToolCallName(msg.Call)
			status := "completed"
			resultText := msg.Result.Content
			if msg.Result.Error != "" {
				status = "error"
				resultText = msg.Result.Error
			}
			var combined strings.Builder
			for _, line := range block.Lines {
				combined.WriteString(line)
				combined.WriteString("\n")
			}
			streamText := strings.TrimSpace(block.StreamText)
			if len(block.Lines) > 0 && streamText != "" {
				combined.WriteString("────────────────────\n")
			}
			if streamText != "" {
				combined.WriteString(streamText)
			} else if resultText != "" {
				combined.WriteString(resultText)
			}
			toolMsg := ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: m.currentRoleDisplay,
				ToolCalls: []ToolCallDisplay{
					{Name: label, Status: status, Duration: duration, Result: strings.TrimSpace(combined.String())},
				},
			}
			m.messages = append(m.messages, toolMsg)
			cmd := m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt)
			return m, tea.Batch(m.printAssistantTranscriptCmd(toolMsg), cmd)
		}
		label := formatToolCallName(msg.Call)
		status := "completed"
		resultText := msg.Result.Content
		if msg.Result.Error != "" {
			status = "error"
			resultText = msg.Result.Error
		}
		toolMsg := ChatMessage{
			Role:     types.RoleAssistant,
			RoleName: m.currentRoleDisplay,
			ToolCalls: []ToolCallDisplay{
				{Name: label, Status: status, Duration: duration, Result: resultText},
			},
		}
		m.messages = append(m.messages, toolMsg)
		cmd := m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt)
		return m, tea.Batch(m.printAssistantTranscriptCmd(toolMsg), cmd)

	case DelegateActivityMsg:
		line := ""
		if msg.Result != nil {
			line = formatDelegateActivityLine(msg.Call, *msg.Result)
		} else {
			line = formatDelegateActivityStart(msg.Call)
		}
		m.appendLiveToolLine(msg.ParentCallID, line)
		if msg.RoleName != "" {
			return m, m.spinner.StartWithStartedAt(fmt.Sprintf("[%s] %s", msg.RoleName, line), m.currentRunStartedAt)
		}
		return m, m.spinner.StartWithStartedAt(line, m.currentRunStartedAt)

	case DelegateStreamMsg:
		m.handleLiveToolStream(msg.ParentCallID, msg.Event)
		m.handleUsageEvent(msg.ParentCallID, msg.Event)
		if msg.Event.Type == types.EventTextDelta && m.autoScroll {
			m.scrollBack = 0
		}
		return m, nil

	case AgentActivityMsg:
		line := ""
		if msg.Result != nil {
			line = formatAgentActivityLine(msg.WorkerLabel, msg.Call, *msg.Result)
		} else {
			line = formatAgentActivityStart(msg.WorkerLabel, msg.Call)
		}
		m.appendLiveToolLine(msg.ParentCallID, line)
		return m, m.spinner.StartWithStartedAt(line, m.currentRunStartedAt)

	case AgentStreamMsg:
		m.handleLiveToolStream(msg.ParentCallID, msg.Event)
		m.handleUsageEvent(msg.ParentCallID, msg.Event)
		if msg.Event.Type == types.EventTextDelta && m.autoScroll {
			m.scrollBack = 0
		}
		return m, nil

	case AgentDoneMsg:
		m.flushEstimatedTokensIfNeeded()
		m.streaming = false
		m.spinner.Stop()
		m.input.Focus()
		var outMsg ChatMessage
		if msg.Error != nil {
			outMsg = ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: m.currentRoleDisplay,
				Duration: formatElapsed(msg.Elapsed),
				Text:     m.theme.ErrorText.Render("Error: " + msg.Error.Error()),
			}
		} else if m.streamBuf != "" {
			outMsg = ChatMessage{
				Role: types.RoleAssistant, RoleName: m.currentRoleDisplay, Duration: formatElapsed(msg.Elapsed), Text: m.streamBuf,
			}
		} else if strings.TrimSpace(msg.Response) != "" {
			outMsg = ChatMessage{
				Role: types.RoleAssistant, RoleName: m.currentRoleDisplay, Duration: formatElapsed(msg.Elapsed), Text: msg.Response,
			}
		} else {
			outMsg = ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: m.currentRoleDisplay,
				Duration: formatElapsed(msg.Elapsed),
				Text:     m.theme.ErrorText.Render("Error: model returned an empty response"),
			}
		}
		m.messages = append(m.messages, outMsg)
		m.totalMsgs = len(m.messages)
		m.streamBuf = ""
		m.lastTurnNeedsSeparator = true
		return m, m.printAssistantTranscriptCmd(outMsg)

	case AllAgentDoneMsg:
		if msg.Usage.TotalTokens <= 0 {
			m.totalTokens += estimateTokens(msg.Response)
		}
		m.addUsage(msg.Usage)
		roleName := msg.RoleName
		var outMsg ChatMessage
		if msg.Error != nil {
			outMsg = ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: roleName,
				Duration: formatElapsed(msg.Elapsed),
				Text:     m.theme.ErrorText.Render("Error: " + msg.Error.Error()),
			}
		} else if msg.Response != "" {
			outMsg = ChatMessage{
				Role: types.RoleAssistant, RoleName: roleName, Duration: formatElapsed(msg.Elapsed), Text: msg.Response,
			}
		} else {
			outMsg = ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: roleName,
				Duration: formatElapsed(msg.Elapsed),
				Text:     m.theme.ErrorText.Render("Error: model returned an empty response"),
			}
		}
		m.messages = append(m.messages, outMsg)
		m.totalMsgs = len(m.messages)
		m.allPending--
		// Always include header for @all — each role needs its own header
		printCmd := printTranscriptCmd(renderTranscriptMessage(m.theme, outMsg, m.width, m.expandTools, m.markdownRender))
		if m.allPending <= 0 {
			m.streaming = false
			m.spinner.Stop()
			m.input.Focus()
			m.lastTurnNeedsSeparator = true
			return m, printCmd
		}
		return m, printCmd

	case SpinnerTickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case pendingSubmitMsg:
		if !m.pendingSubmit || msg.seq != m.pendingSubmitSeq {
			return m, nil
		}
		m.pendingSubmit = false
		return m.submitCurrentInput()
	}

	return m, tea.Batch(cmds...)
}

// ── Agent Execution ─────────────────────────────────────────

func (m *Model) startAgent(a *agent.Agent, input string, active *skill.Skill) (Model, tea.Cmd) {
	if a.Role != nil {
		m.currentRole = a.Role.Name
		m.currentRoleDisplay = a.Role.Config.Name
		if m.currentRoleDisplay == "" {
			m.currentRoleDisplay = a.Role.Name
		}
	}
	m.streaming = true
	m.streamBuf = ""
	m.input.Blur()
	m.currentRunStartedAt = time.Now()
	m.resetTurnUsage()

	ctx, cancel := context.WithCancel(context.Background())
	m.agentCancel = cancel

	cmd := m.spinner.StartWithStartedAt("Thinking...", m.currentRunStartedAt)

	pRef := m.pRef
	startedAt := m.currentRunStartedAt
	go func() {
		a.OnStream = func(event types.StreamEvent) {
			if pRef.p != nil {
				pRef.p.Send(StreamChunkMsg{Event: event})
			}
		}
		a.OnToolCall = func(call types.ToolCall) {
			if pRef.p != nil {
				pRef.p.Send(ToolCallMsg{Call: call})
			}
		}
		a.OnToolResult = func(call types.ToolCall, result types.ToolResult) {
			if pRef.p != nil {
				pRef.p.Send(ToolResultMsg{Call: call, Result: result})
			}
		}
		response, err := skill.RunWithSkill(ctx, a, input, active)
		if pRef.p != nil {
			pRef.p.Send(AgentDoneMsg{Response: response, Error: err, Elapsed: time.Since(startedAt)})
		}
	}()

	return *m, cmd
}

func (m *Model) handleAllBroadcast(input string) (Model, tea.Cmd) {
	agents, cleanInput := m.orchestrator.RouteAll(input)
	if len(agents) == 0 {
		m.messages = append(m.messages, ChatMessage{
			Role: types.RoleAssistant, Text: "Error: no agents available",
		})
		return *m, nil
	}

	m.streaming = true
	m.streamBuf = ""
	m.input.Blur()
	m.allPending = len(agents)
	m.currentRunStartedAt = time.Now()
	m.resetTurnUsage()

	ctx, cancel := context.WithCancel(context.Background())
	m.agentCancel = cancel
	cmd := m.spinner.StartWithStartedAt(fmt.Sprintf("Broadcasting to %d agents...", len(agents)), m.currentRunStartedAt)

	pRef := m.pRef
	activeSkill := m.activeSkill
	var wg sync.WaitGroup
	for _, a := range agents {
		wg.Add(1)
		go func(ag *agent.Agent) {
			defer wg.Done()
			startedAt := time.Now()
			roleName := ag.ID
			if ag.Role != nil {
				roleName = ag.Role.Config.Name
				if roleName == "" {
					roleName = ag.Role.Name
				}
			}
			// Each agent gets isolated memory for this call
			subAgent := ag.CloneIsolated(ag.ID)
			var usage types.Usage
			subAgent.OnStream = func(event types.StreamEvent) {
				if event.Usage != nil {
					usage = *event.Usage
				}
			}
			response, err := skill.RunWithSkill(ctx, subAgent, cleanInput, activeSkill)
			if pRef.p != nil {
				pRef.p.Send(AllAgentDoneMsg{
					RoleName: roleName,
					Response: response,
					Error:    err,
					Elapsed:  time.Since(startedAt),
					Usage:    usage,
				})
			}
		}(a)
	}

	return *m, cmd
}

// ── Mention Popup ───────────────────────────────────────────

func (m *Model) openMention() {
	roles := m.orchestrator.ListAgentRoles()
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	// Add @all as first item
	items := []agent.AgentInfo{{Name: "all", Description: "Broadcast to all agents"}}
	items = append(items, roles...)

	m.mentionItems = items
	m.mentionCursor = 0
	m.showMention = true
	m.filterMention()
}

func (m *Model) filterMention() {
	val := m.input.Value()
	// Extract the @query part
	atIdx := strings.LastIndex(val, "@")
	if atIdx < 0 {
		m.showMention = false
		return
	}
	query := strings.ToLower(val[atIdx+1:])

	roles := m.orchestrator.ListAgentRoles()
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	all := []agent.AgentInfo{{Name: "all", Description: "Broadcast to all agents"}}
	all = append(all, roles...)

	if query == "" {
		m.mentionItems = all
	} else {
		var filtered []agent.AgentInfo
		for _, item := range all {
			if strings.Contains(strings.ToLower(item.Name), query) ||
				strings.Contains(strings.ToLower(item.Description), query) {
				filtered = append(filtered, item)
			}
		}
		m.mentionItems = filtered
	}

	if m.mentionCursor >= len(m.mentionItems) {
		m.mentionCursor = 0
	}
	m.showMention = len(m.mentionItems) > 0
}

func (m *Model) openCommand() {
	m.commandCursor = 0
	m.filterCommand()
}

func (m *Model) filterCommand() {
	val := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(val, "/") {
		m.showCommand = false
		return
	}

	query := strings.ToLower(strings.TrimPrefix(val, "/"))
	all := []CommandInfo{
		{Name: "/clear", Description: "Clear current conversation view"},
		{Name: "/roles", Description: "List available roles"},
		{Name: "/skills", Description: "List available skills"},
		{Name: "/skill show ", Description: "Show one skill package"},
		{Name: "/skill use ", Description: "Activate a skill for later prompts"},
		{Name: "/skill run ", Description: "Run one task with a specific skill"},
		{Name: "/skill clear", Description: "Clear the active skill"},
		{Name: "/quit", Description: "Exit FunCode"},
		{Name: "/exit", Description: "Exit FunCode"},
	}

	if query == "" {
		m.commandItems = all
	} else {
		var filtered []CommandInfo
		for _, item := range all {
			if strings.Contains(strings.ToLower(item.Name), query) ||
				strings.Contains(strings.ToLower(item.Description), query) {
				filtered = append(filtered, item)
			}
		}
		m.commandItems = filtered
	}

	if m.commandCursor >= len(m.commandItems) {
		m.commandCursor = 0
	}
	m.showCommand = len(m.commandItems) > 0
}

// ── /roles Command ──────────────────────────────────────────

func (m Model) buildRolesMessage() []ChatMessage {
	roles := m.orchestrator.ListAgentRoles()
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

	var b strings.Builder
	b.WriteString("Available roles:\n\n")
	for _, r := range roles {
		name := m.theme.AssistantLabel.Render("@" + r.Name)
		desc := ""
		if r.Description != "" {
			desc = "\n    " + m.theme.Dimmed.Render(r.Description)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", name, desc))
	}
	b.WriteString(fmt.Sprintf("\n  %s  %s\n",
		m.theme.AssistantLabel.Render("@all"),
		m.theme.Dimmed.Render("Broadcast to all agents"),
	))

	return []ChatMessage{{
		Role:     types.RoleAssistant,
		RoleName: "system",
		Text:     b.String(),
	}}
}

func (m Model) buildSystemMessage(text string) []ChatMessage {
	return []ChatMessage{{
		Role:     types.RoleAssistant,
		RoleName: "system",
		Text:     text,
	}}
}

func (m Model) handleSkillCommand(cmd skill.Command) (tea.Model, tea.Cmd) {
	m.input.Reset()
	m.showWelcome = false
	m.showMention = false
	m.showCommand = false

	var (
		msgs []ChatMessage
		text string
		err  error
	)

	switch cmd.Action {
	case skill.CommandShow:
		if m.skillManager == nil {
			text = "No skills found."
		} else if cmd.Name == "" {
			text = m.skillManager.BuildListText()
		} else {
			text, err = m.skillManager.BuildShowText(cmd.Name)
		}
	case skill.CommandUse:
		if m.skillManager == nil {
			err = fmt.Errorf("skill %q not found", cmd.Name)
			break
		}
		var exists bool
		m.activeSkill, exists = m.skillManager.Get(cmd.Name)
		if !exists {
			err = fmt.Errorf("skill %q not found", cmd.Name)
			m.activeSkill = nil
			break
		}
		text = fmt.Sprintf("Activated skill: %s\n\n%s", m.activeSkill.Name, m.activeSkill.Description)
	case skill.CommandClear:
		m.activeSkill = nil
		text = "Cleared active skill."
	case skill.CommandRun:
		if m.skillManager == nil {
			err = fmt.Errorf("skill %q not found", cmd.Name)
			break
		}
		s, exists := m.skillManager.Get(cmd.Name)
		if !exists {
			err = fmt.Errorf("skill %q not found", cmd.Name)
			break
		}
		userMsg := ChatMessage{
			Role: types.RoleUser,
			Text: cmd.Task,
		}
		m.messages = append(m.messages, userMsg)
		m.totalMsgs = len(m.messages)

		targetAgent, cleanInput := m.orchestrator.Route(cmd.Task)
		if targetAgent == nil {
			errMsg := ChatMessage{
				Role:     types.RoleAssistant,
				RoleName: "system",
				Text:     "Error: no agent available",
			}
			m.messages = append(m.messages, errMsg)
			m.totalMsgs = len(m.messages)
			return m, tea.Batch(
				printTranscriptCmd(renderTranscriptMessage(m.theme, userMsg, m.width, m.expandTools, m.markdownRender)),
				printTranscriptCmd(renderTranscriptMessage(m.theme, errMsg, m.width, m.expandTools, m.markdownRender)),
			)
		}

		infoMsg := ChatMessage{
			Role:     types.RoleAssistant,
			RoleName: "system",
			Text:     fmt.Sprintf("Running with skill: %s", s.Name),
		}
		m.messages = append(m.messages, infoMsg)
		m.totalMsgs = len(m.messages)

		next, runCmd := m.startAgent(targetAgent, cleanInput, s)
		return next, tea.Batch(
			printTranscriptCmd(renderTranscriptMessage(m.theme, userMsg, m.width, m.expandTools, m.markdownRender)),
			printTranscriptCmd(renderTranscriptMessage(m.theme, infoMsg, m.width, m.expandTools, m.markdownRender)),
			runCmd,
		)
	default:
		text = skill.HelpText(m.activeSkill)
	}

	if err != nil {
		text = "Error: " + err.Error()
	}
	msgs = m.buildSystemMessage(text)
	m.messages = append(m.messages, msgs...)
	m.totalMsgs = len(m.messages)
	return m, printTranscriptCmd(renderTranscriptMessages(m.theme, msgs, m.width, m.expandTools, m.markdownRender))
}

// ── View ────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "  Loading..."
	}

	var b strings.Builder

	sep := m.theme.Separator.Render(strings.Repeat("─", m.width))

	ch := m.contentHeight()
	content := m.renderContent()
	lines := strings.Split(content, "\n")
	if len(lines) > ch {
		if m.autoScroll {
			lines = lines[len(lines)-ch:]
		} else {
			start := len(lines) - ch - m.scrollBack
			if start < 0 {
				start = 0
			}
			end := start + ch
			if end > len(lines) {
				end = len(lines)
			}
			lines = lines[start:end]
		}
	}
	b.WriteString(strings.Join(lines, "\n"))

	// Mention popup
	if m.showMention && len(m.mentionItems) > 0 {
		b.WriteString(m.renderMentionPopup())
		b.WriteString("\n")
	}

	if m.showCommand && len(m.commandItems) > 0 {
		b.WriteString(m.renderCommandPopup())
		b.WriteString("\n")
	}

	// Approval prompt
	if m.showApproval {
		b.WriteString(m.renderApprovalPrompt())
		b.WriteString("\n")
	}

	b.WriteString(sep)
	b.WriteString("\n")

	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	b.WriteString(m.renderStatusLine())

	return b.String()
}

func (m Model) renderMentionPopup() string {
	var items []string
	for i, item := range m.mentionItems {
		name := "@" + item.Name
		desc := ""
		if item.Description != "" {
			desc = " " + m.theme.MentionDim.Render(item.Description)
		}
		line := name + desc
		if i == m.mentionCursor {
			items = append(items, m.theme.MentionSelected.Render(line))
		} else {
			items = append(items, m.theme.MentionItem.Render(line))
		}
	}
	content := strings.Join(items, "\n")
	return m.theme.MentionBox.Render(content)
}

func (m Model) renderCommandPopup() string {
	var items []string
	for i, item := range m.commandItems {
		line := item.Name
		if item.Description != "" {
			line += " " + m.theme.CommandDim.Render(item.Description)
		}
		if i == m.commandCursor {
			items = append(items, m.theme.CommandSelected.Render(line))
		} else {
			items = append(items, m.theme.CommandItem.Render(line))
		}
	}
	return m.theme.CommandBox.Render(strings.Join(items, "\n"))
}

func (m Model) contentHeight() int {
	mentionHeight := 0
	if m.showMention {
		mentionHeight = len(m.mentionItems) + 2
	}
	commandHeight := 0
	if m.showCommand {
		commandHeight = len(m.commandItems) + 2
	}
	approvalHeight := 0
	if m.showApproval {
		approvalHeight = 6
	}
	h := m.height - 5 - mentionHeight - commandHeight - approvalHeight
	if h < 5 {
		h = 5
	}
	return h
}

func (m Model) renderApprovalPrompt() string {
	t := m.theme

	title := t.ToolLabel.Render("  Tool approval required: ") + t.ToolName.Render(m.approvalTool)
	params := m.approvalParams
	if len(params) > 120 {
		params = params[:120] + "..."
	}
	paramsLine := t.Dimmed.Render("  " + params)

	options := []string{"[Y] Allow once", "[A] Always allow", "[N] Deny"}
	var rendered []string
	for i, opt := range options {
		if i == m.approvalCursor {
			rendered = append(rendered, t.ApprovalKey.Render(opt))
		} else {
			rendered = append(rendered, t.Dimmed.Render(opt))
		}
	}
	optLine := "  " + strings.Join(rendered, "  ")
	hint := t.Dimmed.Render("  ←/→ to select, Enter to confirm, Y/A/N shortcut")

	content := title + "\n" + paramsLine + "\n\n" + optLine + "\n" + hint
	return t.ApprovalBox.Width(m.width - 6).Render(content)
}

func (m Model) renderContent() string {
	var b strings.Builder

	if m.showWelcome && len(m.messages) == 0 {
		b.WriteString("\n")
		b.WriteString(m.theme.WelcomeTitle.Render("  Welcome to FunCode!"))
		b.WriteString("\n\n")
		b.WriteString(m.theme.WelcomeText.Render("  Type a message to start chatting."))
		b.WriteString("\n")
		b.WriteString(m.theme.WelcomeText.Render("  Use @role to talk to a specific agent, or @all to broadcast."))
		b.WriteString("\n")
		b.WriteString(m.theme.WelcomeText.Render("  /roles and /skills show capabilities, /clear resets the view, /quit exits."))
		b.WriteString("\n\n")
		return b.String()
	}

	if m.shouldRenderLiveAssistantHeader() {
		b.WriteString(m.renderLiveAssistantHeader())
		b.WriteString("\n")
	}

	if m.streamBuf != "" {
		for _, line := range strings.Split(m.streamBuf, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	for _, block := range m.liveToolBlocks {
		boxWidth := m.width - 8
		if boxWidth < 40 {
			boxWidth = 40
		}
		content := m.theme.ToolName.Render(block.Label) + " " + m.theme.Dimmed.Render("running...")
		if !block.StartedAt.IsZero() {
			content += " " + m.theme.Dimmed.Render("["+formatElapsedSince(block.StartedAt)+"]")
		}
		blockBody := buildLiveToolBlockBody(block)
		if blockBody != "" {
			if m.expandTools {
				content += "\n" + renderFullResult(m.theme, blockBody, boxWidth-4)
			} else {
				content += "\n" + renderCollapsedTailResult(m.theme, blockBody, boxWidth-4, 8)
			}
		}
		b.WriteString(m.theme.ToolBox.Width(boxWidth).Render(content))
		b.WriteString("\n")
	}

	if spin := m.spinner.View(); spin != "" {
		b.WriteString(spin)
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) shouldRenderLiveAssistantHeader() bool {
	if !m.streaming {
		return false
	}
	if m.currentRoleDisplay == "" {
		return false
	}
	return m.streamBuf != "" || len(m.liveToolBlocks) > 0 || m.spinner.active
}

func (m Model) renderLiveAssistantHeader() string {
	msg := ChatMessage{
		Role:     types.RoleAssistant,
		RoleName: m.currentRoleDisplay,
	}
	ts := time.Now()
	if !m.currentRunStartedAt.IsZero() {
		ts = m.currentRunStartedAt
	}
	name := strings.TrimRight(renderMessageHeader(m.theme, msg), "\n")
	return name + " " + m.theme.Dimmed.Render("["+ts.Format("15:04:05")+"]")
}

func renderTranscriptMessage(theme *Theme, msg ChatMessage, width int, expandAll bool, markdownRender bool) string {
	return renderTranscriptMessageWithHeader(theme, msg, width, expandAll, markdownRender, true)
}

func renderTranscriptMessageWithHeader(theme *Theme, msg ChatMessage, width int, expandAll bool, markdownRender bool, includeHeader bool) string {
	body := strings.TrimRight(renderMessageBody(theme, msg, width, expandAll, markdownRender), "\n")
	if !includeHeader {
		return body
	}
	ts := time.Now().Format("15:04:05")
	name := strings.TrimRight(renderMessageHeader(theme, msg), "\n")
	header := name + " " + theme.Dimmed.Render("["+ts+"]")
	if msg.Duration != "" {
		header += " " + theme.Dimmed.Render("["+msg.Duration+"]")
	}
	if body == "" {
		return header
	}
	return header + "\n" + body
}

func renderTranscriptMessages(theme *Theme, msgs []ChatMessage, width int, expandAll bool, markdownRender bool) string {
	var parts []string
	for _, msg := range msgs {
		parts = append(parts, renderTranscriptMessage(theme, msg, width, expandAll, markdownRender))
	}
	return strings.Join(parts, "\n")
}

func printTranscriptCmd(content string) tea.Cmd {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return tea.Printf("%s", content)
}

func (m *Model) printAssistantTranscriptCmd(msg ChatMessage) tea.Cmd {
	includeHeader := !m.turnAssistantHeaderPrinted
	if includeHeader {
		m.turnAssistantHeaderPrinted = true
	}
	return printTranscriptCmd(renderTranscriptMessageWithHeader(m.theme, msg, m.width, m.expandTools, m.markdownRender, includeHeader))
}

func printTurnSeparatorCmd(theme *Theme, width int) tea.Cmd {
	if width <= 0 {
		return nil
	}
	return tea.Printf("%s", theme.Separator.Render(strings.Repeat("─", width)))
}

func buildLiveToolBlockBody(block LiveToolBlock) string {
	var parts []string
	if len(block.Lines) > 0 {
		parts = append(parts, strings.Join(block.Lines, "\n"))
	}
	if strings.TrimSpace(block.StreamText) != "" {
		if len(parts) > 0 {
			parts = append(parts, "────────────────────")
		}
		parts = append(parts, block.StreamText)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func renderCollapsedTailResult(theme *Theme, text string, width int, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return ""
	}
	if maxLines <= 0 {
		maxLines = 8
	}
	if len(lines) <= maxLines {
		return renderFullResult(theme, strings.Join(lines, "\n"), width)
	}
	hidden := len(lines) - maxLines
	visible := strings.Join(lines[hidden:], "\n")
	head := theme.Dimmed.Render(fmt.Sprintf("[+%d lines above · Ctrl+B expand]", hidden))
	return head + "\n" + renderFullResult(theme, visible, width)
}

func (m *Model) resetTurnUsage() {
	if m.activeUsages == nil {
		m.activeUsages = map[string]types.Usage{}
	}
	m.activeUsages = map[string]types.Usage{}
	m.turnEstimatedTokens = 0
	m.turnHasRealUsage = false
}

func (m *Model) handleUsageEvent(key string, event types.StreamEvent) {
	if key == "" {
		return
	}
	if m.activeUsages == nil {
		m.activeUsages = map[string]types.Usage{}
	}
	if event.Usage != nil {
		m.activeUsages[key] = *event.Usage
	}
	if event.Type == types.EventDone {
		if usage, ok := m.activeUsages[key]; ok {
			m.addUsage(usage)
			delete(m.activeUsages, key)
		}
	}
}

func (m *Model) addUsage(usage types.Usage) {
	if usage.TotalTokens <= 0 && usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 {
		return
	}
	m.turnHasRealUsage = true
	m.totalTokens += usage.TotalTokens
}

func (m *Model) addEstimatedTokens(text string) {
	m.turnEstimatedTokens += estimateTokens(text)
}

func (m *Model) flushEstimatedTokensIfNeeded() {
	if m.turnHasRealUsage || len(m.activeUsages) > 0 || m.turnEstimatedTokens <= 0 {
		return
	}
	m.totalTokens += m.turnEstimatedTokens
	m.turnEstimatedTokens = 0
}

func (m Model) renderStatusLine() string {
	t := m.theme
	skillName := "none"
	if m.activeSkill != nil {
		skillName = m.activeSkill.Name
	}

	segments := []string{
		t.Logo.Render("⚙ " + version.Full()),
		t.SlModel.Render("🤖 Model: " + m.modelName),
		t.SlRole.Render("🎭 Role: " + m.currentRoleDisplay),
		t.SlContext.Render("🧠 Skill: " + skillName),
		t.SlContext.Render(fmt.Sprintf("💬 Ctx: %d msgs", m.totalMsgs)),
		t.SlTokens.Render(fmt.Sprintf("🔢 Tokens: %s", formatCompactCount(m.totalTokens))),
		t.SlCwd.Render("📁 " + m.cwd),
	}

	joined := strings.Join(segments, t.SlSep.Render(" | "))
	gap := m.width - lipgloss.Width(joined)
	if gap < 0 {
		gap = 0
	}
	return t.StatusLine.Width(m.width).Render(joined + strings.Repeat(" ", gap))
}

func formatCompactCount(value int) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	if value < 1000000 {
		return trimCompactFloat(float64(value)/1000.0) + "k"
	}
	return trimCompactFloat(float64(value)/1000000.0) + "m"
}

func trimCompactFloat(value float64) string {
	text := fmt.Sprintf("%.1f", value)
	return strings.TrimSuffix(strings.TrimSuffix(text, "0"), ".")
}

// ── Entry Point ─────────────────────────────────────────────

// estimateTokens roughly estimates token count (~4 chars per token)
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

func delayedSubmitCmd(seq int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return pendingSubmitMsg{seq: seq}
	})
}

func looksLikePaste(msg tea.KeyMsg) bool {
	if msg.Paste {
		return true
	}
	if msg.Type != tea.KeyRunes {
		return false
	}
	text := string(msg.Runes)
	if strings.ContainsAny(text, "\r\n\t") {
		return true
	}
	return len(msg.Runes) >= 8
}

func (m Model) submitCurrentInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}
	m.pushHistory(input)

	if input == "/quit" || input == "/exit" {
		return m, tea.Quit
	}
	if input == "/clear" {
		m.messages = nil
		m.input.Reset()
		m.showWelcome = true
		return m, nil
	}
	if input == "/roles" {
		m.input.Reset()
		m.showWelcome = false
		m.messages = append(m.messages, m.buildRolesMessage()...)
		m.totalMsgs = len(m.messages)
		return m, printTranscriptCmd(renderTranscriptMessages(m.theme, m.buildRolesMessage(), m.width, m.expandTools, m.markdownRender))
	}
	if cmd, ok := skill.ParseCommand(input); ok {
		return m.handleSkillCommand(cmd)
	}

	m.input.Reset()
	m.showWelcome = false
	m.showMention = false
	m.showCommand = false
	m.turnAssistantHeaderPrinted = false

	var sepCmd tea.Cmd
	if m.lastTurnNeedsSeparator {
		m.lastTurnNeedsSeparator = false
		sepCmd = printTurnSeparatorCmd(m.theme, m.width)
	}

	userMsg := ChatMessage{
		Role: types.RoleUser,
		Text: input,
	}
	m.messages = append(m.messages, userMsg)
	m.totalMsgs = len(m.messages)

	if m.orchestrator.IsAllMention(input) {
		next, cmd := m.handleAllBroadcast(input)
		printCmd := printTranscriptCmd(renderTranscriptMessage(m.theme, userMsg, m.width, m.expandTools, m.markdownRender))
		if sepCmd != nil {
			return next, tea.Batch(tea.Sequence(sepCmd, printCmd), cmd)
		}
		return next, tea.Batch(printCmd, cmd)
	}

	targetAgent, cleanInput := m.orchestrator.Route(input)
	if targetAgent == nil {
		errMsg := ChatMessage{
			Role: types.RoleAssistant,
			Text: "Error: no agent available",
		}
		m.messages = append(m.messages, errMsg)
		return m, printTranscriptCmd(renderTranscriptMessage(m.theme, errMsg, m.width, m.expandTools, m.markdownRender))
	}

	next, cmd := m.startAgent(targetAgent, cleanInput, m.activeSkill)
	printCmd := printTranscriptCmd(renderTranscriptMessage(m.theme, userMsg, m.width, m.expandTools, m.markdownRender))
	if sepCmd != nil {
		return next, tea.Batch(tea.Sequence(sepCmd, printCmd), cmd)
	}
	return next, tea.Batch(printCmd, cmd)
}

func (m *Model) pushHistory(input string) {
	if input == "" {
		return
	}
	if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != input {
		m.inputHistory = append(m.inputHistory, input)
	}
	m.historyCursor = -1
	m.historyDraft = ""
}

func (m *Model) historyUp() {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyCursor == -1 {
		m.historyDraft = m.input.Value()
		m.historyCursor = len(m.inputHistory) - 1
	} else if m.historyCursor > 0 {
		m.historyCursor--
	}
	m.input.SetValue(m.inputHistory[m.historyCursor])
}

func (m *Model) historyDown() {
	if len(m.inputHistory) == 0 || m.historyCursor == -1 {
		return
	}
	if m.historyCursor < len(m.inputHistory)-1 {
		m.historyCursor++
		m.input.SetValue(m.inputHistory[m.historyCursor])
		return
	}
	m.historyCursor = -1
	m.input.SetValue(m.historyDraft)
}

func formatToolCallName(call types.ToolCall) string {
	switch call.Name {
	case "Delegate":
		var payload struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal([]byte(call.Params), &payload); err == nil && payload.Role != "" {
			return "Delegate -> " + payload.Role
		}
	case "Collaborate":
		var payload struct {
			Tasks []struct {
				Role string `json:"role"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal([]byte(call.Params), &payload); err == nil && len(payload.Tasks) > 0 {
			roleSet := map[string]bool{}
			var roles []string
			for _, task := range payload.Tasks {
				if task.Role == "" || roleSet[task.Role] {
					continue
				}
				roleSet[task.Role] = true
				roles = append(roles, task.Role)
			}
			if len(roles) > 0 {
				return "Collaborate -> " + strings.Join(roles, ", ")
			}
		}
	case "Agent":
		var payload struct {
			Description string `json:"description"`
			Task        string `json:"task"`
		}
		if err := json.Unmarshal([]byte(call.Params), &payload); err == nil {
			label := payload.Description
			if label == "" && len(payload.Task) > 40 {
				label = payload.Task[:40] + "..."
			} else if label == "" {
				label = payload.Task
			}
			return "Agent: " + label
		}
	case "RunTask":
		var payload struct {
			Task string `json:"task"`
		}
		if err := json.Unmarshal([]byte(call.Params), &payload); err == nil && payload.Task != "" {
			return "RunTask: " + payload.Task
		}
		return "RunTask"
	}
	return call.Name
}

func isFileToolCall(call types.ToolCall) bool {
	switch call.Name {
	case "Tree", "Read", "ReadRange", "Glob", "Grep", "Diff", "Edit", "Patch", "Write":
		return true
	default:
		return false
	}
}

func isDelegateToolCall(call types.ToolCall) bool {
	return call.Name == "Delegate"
}

func isAgentToolCall(call types.ToolCall) bool {
	return call.Name == "Agent"
}

func formatAgentActivityStart(workerLabel string, call types.ToolCall) string {
	detail := summarizeFileToolCall(call)
	if detail == "" || detail == call.Name+"..." {
		detail = call.Name
	}
	if workerLabel == "" {
		return detail
	}
	return "[" + workerLabel + "] " + detail
}

func formatDelegateActivityStart(call types.ToolCall) string {
	detail := summarizeFileToolCall(call)
	if detail == "" || detail == call.Name+"..." {
		detail = call.Name
	}
	return detail
}

func formatAgentActivityLine(workerLabel string, call types.ToolCall, result types.ToolResult) string {
	line := formatDelegateActivityLine(call, result)
	if workerLabel == "" {
		return line
	}
	return "[" + workerLabel + "] " + line
}

func (m *Model) rememberToolStart(callID string) time.Time {
	if callID == "" {
		return time.Now()
	}
	if m.toolStartTimes == nil {
		m.toolStartTimes = map[string]time.Time{}
	}
	if startedAt, ok := m.toolStartTimes[callID]; ok {
		return startedAt
	}
	startedAt := time.Now()
	m.toolStartTimes[callID] = startedAt
	return startedAt
}

func (m *Model) finishTool(callID string) time.Duration {
	if callID == "" || m.toolStartTimes == nil {
		return 0
	}
	startedAt, ok := m.toolStartTimes[callID]
	if !ok {
		return 0
	}
	delete(m.toolStartTimes, callID)
	return time.Since(startedAt)
}

func (m *Model) addLiveToolBlock(callID string, label string, startedAt time.Time) {
	if callID == "" {
		return
	}
	m.liveToolBlocks = append(m.liveToolBlocks, LiveToolBlock{
		CallID:    callID,
		Label:     label,
		StartedAt: startedAt,
	})
}

func (m *Model) appendLiveToolLine(callID string, line string) {
	if callID == "" || strings.TrimSpace(line) == "" {
		return
	}
	for i := range m.liveToolBlocks {
		if m.liveToolBlocks[i].CallID == callID {
			m.liveToolBlocks[i].Lines = append(m.liveToolBlocks[i].Lines, line)
			return
		}
	}
}

func (m *Model) appendLiveToolStream(callID string, text string) {
	if callID == "" || text == "" {
		return
	}
	for i := range m.liveToolBlocks {
		if m.liveToolBlocks[i].CallID == callID {
			m.liveToolBlocks[i].StreamText += text
			return
		}
	}
}

func (m *Model) appendLiveToolError(callID string, errText string) {
	if callID == "" || strings.TrimSpace(errText) == "" {
		return
	}
	line := "Error: " + errText
	for i := range m.liveToolBlocks {
		if m.liveToolBlocks[i].CallID == callID {
			if strings.TrimSpace(m.liveToolBlocks[i].StreamText) != "" && !strings.HasSuffix(m.liveToolBlocks[i].StreamText, "\n") {
				m.liveToolBlocks[i].StreamText += "\n"
			}
			m.liveToolBlocks[i].StreamText += line
			return
		}
	}
}

func (m *Model) handleLiveToolStream(callID string, event types.StreamEvent) {
	switch event.Type {
	case types.EventTextDelta:
		m.appendLiveToolStream(callID, event.Content)
		m.addEstimatedTokens(event.Content)
	case types.EventError:
		m.appendLiveToolError(callID, event.Error)
	}
}

func (m *Model) removeLiveToolBlock(callID string) LiveToolBlock {
	for i := range m.liveToolBlocks {
		if m.liveToolBlocks[i].CallID == callID {
			block := m.liveToolBlocks[i]
			block.Lines = append([]string(nil), block.Lines...)
			m.liveToolBlocks = append(m.liveToolBlocks[:i], m.liveToolBlocks[i+1:]...)
			return block
		}
	}
	return LiveToolBlock{}
}

// formatDelegateActivityLine formats a one-line summary for a delegate sub-agent's tool activity
func formatDelegateActivityLine(call types.ToolCall, result types.ToolResult) string {
	if result.Error != "" {
		return fmt.Sprintf("%s error: %s", call.Name, truncateLine(result.Error, 60))
	}
	switch call.Name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return fmt.Sprintf("Read %s (%d lines)", shortenPath(p.FilePath), lines)
	case "ReadRange":
		var p struct {
			FilePath  string `json:"file_path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return fmt.Sprintf("ReadRange %s (%d-%d, %d lines)", shortenPath(p.FilePath), p.StartLine, p.EndLine, lines)
	case "Tree":
		var p struct {
			Path string `json:"path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		if p.Path == "" {
			p.Path = "."
		}
		return fmt.Sprintf("Tree %s (%d lines)", shortenPath(p.Path), lines)
	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return fmt.Sprintf("Glob %s (%d results)", p.Pattern, lines)
	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return fmt.Sprintf("Grep %q (%d results)", p.Pattern, lines)
	case "Diff":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		target := p.FilePath
		if target == "" {
			target = "."
		}
		_, lines := getPreviewLine(result.Content)
		return fmt.Sprintf("Diff %s (%d lines)", shortenPath(target), lines)
	case "Edit":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		return fmt.Sprintf("Edit %s", shortenPath(p.FilePath))
	case "Patch":
		return "Patch applied"
	case "RunTask":
		var p struct {
			Task string `json:"task"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		if p.Task != "" {
			return fmt.Sprintf("RunTask %s", p.Task)
		}
		return "RunTask completed"
	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		return fmt.Sprintf("Write %s", shortenPath(p.FilePath))
	case "Bash":
		var p struct {
			Command string `json:"command"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		cmd := truncateLine(p.Command, 50)
		return fmt.Sprintf("Bash: %s", cmd)
	case "Delete":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		return fmt.Sprintf("Delete %s", shortenPath(p.FilePath))
	default:
		return fmt.Sprintf("%s done", call.Name)
	}
}

func summarizeFileToolCall(call types.ToolCall) string {
	switch call.Name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			return "Reading " + shortenPath(p.FilePath)
		}
	case "ReadRange":
		var p struct {
			FilePath  string `json:"file_path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			if p.EndLine > 0 {
				return fmt.Sprintf("Reading %s lines %d-%d", shortenPath(p.FilePath), p.StartLine, p.EndLine)
			}
			return fmt.Sprintf("Reading %s from line %d", shortenPath(p.FilePath), p.StartLine)
		}
	case "Tree":
		var p struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			if p.Path == "" {
				return "Listing project tree"
			}
			return "Listing tree under " + shortenPath(p.Path)
		}
	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			if p.Path != "" {
				return fmt.Sprintf("Listing %s in %s", p.Pattern, shortenPath(p.Path))
			}
			return fmt.Sprintf("Listing %s", p.Pattern)
		}
	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Glob    string `json:"glob"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			detail := fmt.Sprintf("Searching %q", p.Pattern)
			if p.Glob != "" {
				detail += " in " + p.Glob
			}
			if p.Path != "" {
				detail += " under " + shortenPath(p.Path)
			}
			return detail
		}
	case "Diff":
		var p struct {
			FilePath string `json:"file_path"`
			Staged   bool   `json:"staged"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			detail := "Showing diff"
			if p.Staged {
				detail = "Showing staged diff"
			}
			if p.FilePath != "" {
				detail += " for " + shortenPath(p.FilePath)
			}
			return detail
		}
	case "Edit":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			return "Editing " + shortenPath(p.FilePath)
		}
	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal([]byte(call.Params), &p) == nil {
			return "Writing " + shortenPath(p.FilePath)
		}
	case "Patch":
		return "Applying patch"
	}
	return call.Name + "..."
}

func summarizeFileToolResult(theme *Theme, call types.ToolCall, result types.ToolResult, duration string) string {
	withDuration := func(text string) string {
		if duration == "" {
			return text
		}
		return text + " " + theme.Dimmed.Render("["+duration+"]")
	}
	if result.Error != "" {
		return withDuration(theme.ToolErr.Render(fmt.Sprintf("  %s %s error: %s", call.Name, extractFilePath(call), truncateLine(result.Error, 80))))
	}

	switch call.Name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Read %s (%d lines)", shortenPath(p.FilePath), lines)))

	case "ReadRange":
		var p struct {
			FilePath  string `json:"file_path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		if p.EndLine > 0 {
			return withDuration(theme.Dimmed.Render(fmt.Sprintf("  ReadRange %s (%d-%d, %d lines)", shortenPath(p.FilePath), p.StartLine, p.EndLine, lines)))
		}
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  ReadRange %s (from %d, %d lines)", shortenPath(p.FilePath), p.StartLine, lines)))

	case "Tree":
		var p struct {
			Path string `json:"path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		if p.Path == "" {
			p.Path = "."
		}
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Tree %s (%d lines)", shortenPath(p.Path), lines)))

	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		label := p.Pattern
		if p.Path != "" {
			label += " in " + shortenPath(p.Path)
		}
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Glob %s (%d results)", label, lines)))

	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Grep %q (%d results)", p.Pattern, lines)))

	case "Diff":
		var p struct {
			FilePath string `json:"file_path"`
			Staged   bool   `json:"staged"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		target := p.FilePath
		if target == "" {
			target = "."
		}
		label := "Diff"
		if p.Staged {
			label = "Diff (staged)"
		}
		_, lines := getPreviewLine(result.Content)
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  %s %s (%d lines)", label, shortenPath(target), lines)))

	case "Edit":
		var p struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
			LineStart int    `json:"line_start"`
			LineEnd   int    `json:"line_end"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		header := theme.Dimmed.Render(fmt.Sprintf("  Edit %s", shortenPath(p.FilePath)))
		var diff string
		if p.LineStart > 0 && p.LineEnd > 0 {
			diff = theme.Dimmed.Render(fmt.Sprintf("    lines %d-%d -> %q", p.LineStart, p.LineEnd, truncateLine(strings.ReplaceAll(p.NewString, "\n", "\\n"), 60)))
		} else {
			old := truncateLine(strings.ReplaceAll(p.OldString, "\n", "\\n"), 50)
			new := truncateLine(strings.ReplaceAll(p.NewString, "\n", "\\n"), 50)
			diff = theme.Dimmed.Render(fmt.Sprintf("    %q -> %q", old, new))
		}
		return withDuration(header + "\n" + diff)

	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		json.Unmarshal([]byte(call.Params), &p)
		_, lines := getPreviewLine(result.Content)
		if lines <= 0 {
			lines = strings.Count(result.Content, "\n") + 1
		}
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Write %s (%d lines)", shortenPath(p.FilePath), lines)))

	case "Patch":
		_, lines := getPreviewLine(result.Content)
		return withDuration(theme.Dimmed.Render(fmt.Sprintf("  Patch applied (%d lines)", lines)))
	}

	return withDuration(theme.Dimmed.Render(fmt.Sprintf("  %s done", call.Name)))
}

func extractFilePath(call types.ToolCall) string {
	var p struct {
		FilePath string `json:"file_path"`
	}
	if json.Unmarshal([]byte(call.Params), &p) == nil {
		return shortenPath(p.FilePath)
	}
	return ""
}

func truncateLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func shortenPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	path = strings.ReplaceAll(path, "\\", "/")
	if len(path) <= 72 {
		return path
	}
	return "..." + path[len(path)-69:]
}

func Run(orchestrator *agent.Orchestrator, modelName string, markdownRender bool, skillManager *skill.Manager, extRef *ProgramRef) error {
	m := New(orchestrator, modelName, markdownRender, skillManager)
	p := tea.NewProgram(m, tea.WithMouseCellMotion())
	m.pRef.p = p
	if extRef != nil {
		extRef.P = p
	}
	_, err := p.Run()
	return err
}
