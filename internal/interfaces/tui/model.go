package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"

	"github.com/spacexc/grok-build/internal/application/agent"
	sessionapp "github.com/spacexc/grok-build/internal/application/session"
	domainsession "github.com/spacexc/grok-build/internal/domain/session"
	"github.com/spacexc/grok-build/pkg/config"
)

// Model represents the TUI application state.
type Model struct {
	cfg        *config.Config
	logger     *logrus.Logger
	agent      *agent.Agent
	sessionSvc *sessionapp.Service

	viewport   viewport.Model
	textarea   textarea.Model
	mdRenderer *glamour.TermRenderer

	ready      bool
	width      int
	height     int
	sessionID  string
	workDir    string
	messages   []*domainsession.Message
	streaming  bool
	agentState string
	statusMsg  string
	tokenCount int
	toolCount  int
	quitting   bool

	// Streaming internals
	streamCh chan string
	errCh    chan error
}

// NewModel creates a new TUI model.
func NewModel(
	cfg *config.Config,
	logger *logrus.Logger,
	ag *agent.Agent,
	sessionSvc *sessionapp.Service,
	workDir string,
) *Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Esc to quit)"
	ta.CharLimit = 8000
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Prompt = ""

	// 柔和风格的输入框样式
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8b7eac"))
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#8b7eac"))
	ta.FocusedStyle.Text = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#d4cfe0"))
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6b5e8c"))
	ta.BlurredStyle.Text = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a098b8"))
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5a5068"))
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5a5068"))

	ta.Focus() // 让输入框获得焦点，否则无法接收键盘输入

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().Padding(0, 1)

	mr, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	return &Model{
		cfg:        cfg,
		logger:     logger,
		agent:      ag,
		sessionSvc: sessionSvc,
		viewport:   vp,
		textarea:   ta,
		mdRenderer: mr,
		workDir:    workDir,
		agentState: "idle",
		statusMsg:  "Starting...",
		messages:   make([]*domainsession.Message, 0),
	}
}

// Init returns the initial commands.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.createSessionCmd(),
	)
}

// --- Messages ---

type sessionCreatedMsg struct {
	sessionID string
	workDir   string
	err       error
}

type streamChunkMsg struct {
	content string
}

type streamDoneMsg struct {
	err error
}

type tickMsg struct{}

// --- Commands ---

func (m *Model) createSessionCmd() tea.Cmd {
	return func() tea.Msg {
		sess, err := m.sessionSvc.CreateSession(m.workDir)
		if err != nil {
			return sessionCreatedMsg{err: err}
		}
		return sessionCreatedMsg{sessionID: sess.ID, workDir: sess.WorkDir}
	}
}

func (m *Model) sendPromptCmd(prompt string) tea.Cmd {
	return func() tea.Msg {
		m.streamCh = make(chan string, 100)
		m.errCh = make(chan error, 1)

		go func() {
			_, err := m.agent.Run(context.Background(), m.sessionID, prompt, m.streamCh)
			if err != nil {
				select {
				case m.errCh <- err:
				default:
				}
			}
			close(m.streamCh)
		}()

		// Wait for the first chunk or error
		select {
		case chunk, ok := <-m.streamCh:
			if ok {
				return streamChunkMsg{content: chunk}
			}
			select {
			case err := <-m.errCh:
				return streamDoneMsg{err: err}
			default:
				return streamDoneMsg{}
			}
		case err := <-m.errCh:
			return streamDoneMsg{err: err}
		}
	}
}

// listenForNext returns a command that reads the next chunk from the stream.
func (m *Model) listenForNextCmd() tea.Cmd {
	return func() tea.Msg {
		if m.streamCh == nil {
			return streamDoneMsg{}
		}
		select {
		case chunk, ok := <-m.streamCh:
			if ok {
				return streamChunkMsg{content: chunk}
			}
			// Channel closed, check for error
			select {
			case err := <-m.errCh:
				return streamDoneMsg{err: err}
			default:
				return streamDoneMsg{}
			}
		case err := <-m.errCh:
			return streamDoneMsg{err: err}
		}
	}
}

// --- Update ---

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		cmds  []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.streaming {
				m.streaming = false
				m.agentState = "idle"
				m.statusMsg = "Cancelled"
				m.streamCh = nil
				m.errCh = nil
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if !m.streaming && m.sessionID != "" {
				userInput := strings.TrimSpace(m.textarea.Value())
				if userInput == "" {
					return m, nil
				}
				m.addMessage("user", userInput)
				m.textarea.Reset()
				m.streaming = true
				m.agentState = "thinking"
				m.statusMsg = "Thinking..."
				cmds = append(cmds, m.sendPromptCmd(userInput))
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		footerHeight := 3
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
		m.textarea.SetWidth(msg.Width - 4)
		if !m.ready {
			m.ready = true
		}

	case sessionCreatedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		m.sessionID = msg.sessionID
		m.workDir = msg.workDir
		m.statusMsg = fmt.Sprintf("Session: %s", truncateStr(m.sessionID, 12))
		m.logger.WithField("session_id", m.sessionID).Info("Session created")

	case streamChunkMsg:
		// Append to the last assistant message or create a new one
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == domainsession.RoleAssistant {
			m.messages[len(m.messages)-1].Content += msg.content
		} else {
			m.addMessage("assistant", msg.content)
		}
		m.tokenCount += len(msg.content) / 4
		m.updateViewport()
		// Continue listening for more chunks
		return m, m.listenForNextCmd()

	case streamDoneMsg:
		m.streaming = false
		m.agentState = "idle"
		m.streamCh = nil
		m.errCh = nil
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			m.addMessage("system", "Error: "+msg.err.Error())
		} else {
			m.statusMsg = fmt.Sprintf("Done | tokens: ~%d | tools: %d", m.tokenCount, m.toolCount)
		}
		m.updateViewport()

	case tickMsg:
		// no-op: just re-renders

	case error:
		m.logger.WithError(msg).Error("TUI error")
		return m, nil
	}

	m.textarea, taCmd = m.textarea.Update(msg)
	cmds = append(cmds, taCmd)

	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// --- View ---

func (m *Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}
	if m.quitting {
		return "Goodbye!\n"
	}

	// 柔和配色方案
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#9b8ec4")).
		Padding(0, 1).
		Width(m.width)

	title := titleStyle.Render("SpaceXC Grok Build (Go)")

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#b8a9d4")).
		Background(lipgloss.Color("#2a2533")).
		Padding(0, 1).
		Width(m.width)

	status := statusStyle.Render(m.statusMsg)

	inputStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#8b7eac")).
		BorderTop(true).
		Padding(0, 1).
		Width(m.width)

	input := inputStyle.Render(m.textarea.View())

	return lipgloss.JoinVertical(
		lipgloss.Top,
		title,
		m.viewport.View(),
		status,
		input,
	)
}

// --- Helpers ---

func (m *Model) addMessage(role, content string) {
	msg := &domainsession.Message{
		Role:      domainsession.Role(role),
		Content:   content,
		CreatedAt: time.Now(),
	}
	m.messages = append(m.messages, msg)
}

func (m *Model) updateViewport() {
	var sb strings.Builder
	userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7ba5c8")).Bold(true)
	assistantStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7db89a")).Bold(true)
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c9a96e")).Bold(true)
	systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#d4858a")).Bold(true)

	for _, msg := range m.messages {
		switch msg.Role {
		case domainsession.RoleUser:
			sb.WriteString(userStyle.Render("You:"))
			sb.WriteString("\n")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		case domainsession.RoleAssistant:
			sb.WriteString(assistantStyle.Render("Assistant:"))
			sb.WriteString("\n")
			rendered, err := m.mdRenderer.Render(msg.Content)
			if err != nil {
				sb.WriteString(msg.Content)
			} else {
				sb.WriteString(rendered)
			}
			sb.WriteString("\n\n")
		case domainsession.RoleTool:
			sb.WriteString(toolStyle.Render("[Tool]"))
			sb.WriteString("\n")
			rendered, err := m.mdRenderer.Render(msg.Content)
			if err != nil {
				sb.WriteString(msg.Content)
			} else {
				sb.WriteString(rendered)
			}
			sb.WriteString("\n\n")
		case domainsession.RoleSystem:
			sb.WriteString(systemStyle.Render("[System]"))
			sb.WriteString(" ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")
		}
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}