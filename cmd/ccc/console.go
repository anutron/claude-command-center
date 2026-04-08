package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/ui"
)

// consoleModel is the bubbletea model for the standalone console TUI.
type consoleModel struct {
	client      *daemon.Client
	entries     []db.AgentHistoryEntry
	llmActivity []daemon.LLMActivityEvent
	cursor      int
	width       int
	height      int
	events      []agent.SessionEvent
	done        bool
}

// consolePollMsg is sent on each tick to trigger a data refresh.
type consolePollMsg struct{}

func consoleTick() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return consolePollMsg{}
	})
}

func (m consoleModel) Init() tea.Cmd {
	return consoleTick()
}

func (m consoleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				return m, m.fetchSelected()
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
				return m, m.fetchSelected()
			}
		}
		return m, nil

	case consolePollMsg:
		return m, tea.Batch(m.fetchHistory(), m.fetchLLMActivity(), consoleTick())

	case consoleHistoryMsg:
		m.entries = msg.entries
		// Keep cursor in bounds.
		if m.cursor >= len(m.entries) && len(m.entries) > 0 {
			m.cursor = len(m.entries) - 1
		}
		// Also refresh the selected agent's output.
		return m, m.fetchSelected()

	case consoleOutputMsg:
		m.events = msg.events
		m.done = msg.done
		return m, nil

	case consoleLLMActivityMsg:
		m.llmActivity = msg.events
		return m, nil
	}

	return m, nil
}

// consoleHistoryMsg carries a refreshed agent history list.
type consoleHistoryMsg struct {
	entries []db.AgentHistoryEntry
}

// consoleOutputMsg carries refreshed events for the selected agent.
type consoleOutputMsg struct {
	events []agent.SessionEvent
	done   bool
}

// consoleLLMActivityMsg carries refreshed LLM activity events.
type consoleLLMActivityMsg struct {
	events []daemon.LLMActivityEvent
}

func (m consoleModel) fetchHistory() tea.Cmd {
	return func() tea.Msg {
		entries, err := m.client.ListAgentHistory(24)
		if err != nil {
			return consoleHistoryMsg{entries: m.entries} // keep old on error
		}
		return consoleHistoryMsg{entries: entries}
	}
}

func (m consoleModel) fetchLLMActivity() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return consoleLLMActivityMsg{}
		}
		events, err := m.client.ListLLMActivity()
		if err != nil {
			return consoleLLMActivityMsg{events: m.llmActivity}
		}
		return consoleLLMActivityMsg{events: events}
	}
}

func (m consoleModel) fetchSelected() tea.Cmd {
	if len(m.entries) == 0 {
		return nil
	}
	agentID := m.entries[m.cursor].AgentID
	return func() tea.Msg {
		result, err := m.client.StreamAgentOutput(agentID)
		if err != nil {
			return consoleOutputMsg{events: m.events, done: m.done}
		}
		return consoleOutputMsg{events: result.Events, done: result.Done}
	}
}

// View renders the full console layout.
func (m consoleModel) View() string {
	if m.width == 0 {
		return ""
	}

	sidebarWidth := 28
	if m.width < 60 {
		sidebarWidth = 20
	}
	if sidebarWidth >= m.width {
		sidebarWidth = m.width / 3
	}
	focusWidth := m.width - sidebarWidth - 1 // 1 for border

	sidebar := m.renderSidebar(sidebarWidth)
	focus := m.renderFocus(focusWidth)

	sidebarStyle := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(m.height).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#3b4261"))

	focusStyle := lipgloss.NewStyle().
		Width(focusWidth).
		Height(m.height)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarStyle.Render(sidebar),
		focusStyle.Render(focus),
	)
}

// isActive returns true if the status represents an agent that is still running.
func isActive(status string) bool {
	switch status {
	case "running", "processing", "queued", "blocked":
		return true
	}
	return false
}

func (m consoleModel) renderSidebar(width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7aa2f7"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("#283457")).Width(width)

	var lines []string
	lines = append(lines, titleStyle.Render("AGENTS"))
	lines = append(lines, "")

	if len(m.entries) == 0 && len(m.llmActivity) == 0 {
		lines = append(lines, dimStyle.Render("No agents running"))
		lines = append(lines, dimStyle.Render("Watching..."))
		return strings.Join(lines, "\n")
	}

	// Collect active and completed separately.
	var activeIdx, completedIdx []int
	for i, e := range m.entries {
		if isActive(e.Status) {
			activeIdx = append(activeIdx, i)
		} else {
			completedIdx = append(completedIdx, i)
		}
	}

	maxLabel := width - 4 // icon + space + padding
	if maxLabel < 5 {
		maxLabel = 5
	}

	renderItem := func(i int, dimmed bool) string {
		e := m.entries[i]
		icon := ui.AgentStatusIcon(e.Status)
		color := ui.AgentStatusColor(e.Status)

		label := e.OriginLabel
		if utf8.RuneCountInString(label) > maxLabel {
			runes := []rune(label)
			label = string(runes[:maxLabel-1]) + "…"
		}

		iconStyle := lipgloss.NewStyle().Foreground(color)
		if dimmed {
			iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
		}
		textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
		if dimmed {
			textStyle = dimStyle
		}

		row := iconStyle.Render(icon) + " " + textStyle.Render(label)
		if i == m.cursor {
			row = selectedBg.Render(row)
		}
		return row
	}

	for _, i := range activeIdx {
		lines = append(lines, renderItem(i, false))
	}

	if len(completedIdx) > 0 {
		sep := dimStyle.Render("── completed ──")
		lines = append(lines, sep)
		for _, i := range completedIdx {
			lines = append(lines, renderItem(i, true))
		}
	}

	// LLM activity section
	if len(m.llmActivity) > 0 {
		lines = append(lines, dimStyle.Render("── llm ──"))
		for _, evt := range m.llmActivity {
			icon := "●"
			color := lipgloss.Color("#565f89")
			switch evt.Status {
			case "running":
				icon = "◐"
				color = lipgloss.Color("#e0af68")
			case "completed":
				icon = "✓"
				color = lipgloss.Color("#9ece6a")
			case "failed":
				icon = "✗"
				color = lipgloss.Color("#f7768e")
			}
			iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)
			label := evt.Operation
			if evt.DurationMs > 0 {
				secs := evt.DurationMs / 1000
				if secs > 0 {
					label += fmt.Sprintf(" %ds", secs)
				} else {
					label += fmt.Sprintf(" %dms", evt.DurationMs)
				}
			} else if evt.Status == "running" {
				elapsed := time.Since(evt.StartedAt).Truncate(time.Second)
				label += fmt.Sprintf(" %s", elapsed)
			}
			lines = append(lines, iconStyled+" "+dimStyle.Render(label))
		}
	}

	return strings.Join(lines, "\n")
}

func (m consoleModel) renderFocus(width int) string {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	blueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))

	if len(m.entries) == 0 {
		return dimStyle.Render("No agents running. Watching for activity...")
	}

	e := m.entries[m.cursor]
	statusColor := ui.AgentStatusColor(e.Status)
	statusStyle := lipgloss.NewStyle().Foreground(statusColor)

	var lines []string

	// Header: icon + label.
	header := statusStyle.Render(ui.AgentStatusIcon(e.Status)) + " " +
		statusStyle.Render(e.OriginLabel)
	lines = append(lines, header)

	// Meta: elapsed + cost.
	elapsed := ui.FormatAgentElapsed(e)
	cost := ""
	if e.CostUSD > 0 {
		cost = fmt.Sprintf("  $%.4f", e.CostUSD)
	}
	lines = append(lines, dimStyle.Render(elapsed+cost))

	// Separator.
	sep := strings.Repeat("─", width)
	lines = append(lines, dimStyle.Render(sep))

	// Events.
	if len(m.events) == 0 {
		if m.done {
			lines = append(lines, dimStyle.Render("(no event data available)"))
		} else {
			lines = append(lines, dimStyle.Render("Waiting for events..."))
		}
	} else {
		// Available rows for events: height minus header lines.
		headerLines := 3 // header + meta + separator
		maxRows := m.height - headerLines - 1
		if maxRows < 1 {
			maxRows = 1
		}

		var eventLines []string
		for _, ev := range m.events {
			var line string
			switch ev.Type {
			case "tool_use":
				input := ev.ToolInput
				if utf8.RuneCountInString(input) > 60 {
					runes := []rune(input)
					input = string(runes[:59]) + "…"
				}
				line = "⠋ " + blueStyle.Render(ev.ToolName) + " " + dimStyle.Render(input)
			case "tool_result":
				if ev.IsError {
					text := ev.ResultText
					if utf8.RuneCountInString(text) > 80 {
						runes := []rune(text)
						text = string(runes[:79]) + "…"
					}
					line = redStyle.Render("✗ " + text)
				} else {
					text := ev.ResultText
					if utf8.RuneCountInString(text) > 80 {
						runes := []rune(text)
						text = string(runes[:79]) + "…"
					}
					line = dimStyle.Render("→ " + text)
				}
			case "assistant_text":
				text := ev.Text
				if utf8.RuneCountInString(text) > 80 {
					runes := []rune(text)
					text = string(runes[:79]) + "…"
				}
				line = text
			case "error":
				line = redStyle.Render("ERROR: " + ev.Text)
			default:
				continue
			}
			eventLines = append(eventLines, line)
		}

		// Auto-scroll: show only the last maxRows lines.
		if len(eventLines) > maxRows {
			eventLines = eventLines[len(eventLines)-maxRows:]
		}
		lines = append(lines, eventLines...)
	}

	return strings.Join(lines, "\n")
}

// runConsole is the entry point for `ccc console`.
func runConsole(_ []string) error {
	sockPath := filepath.Join(config.ConfigDir(), "daemon.sock")
	client, err := daemon.NewClient(sockPath)
	if err != nil {
		return fmt.Errorf("could not connect to daemon at %s: %w\nIs the daemon running? Try: ccc daemon start", sockPath, err)
	}
	defer client.Close()

	m := consoleModel{
		client: client,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
