package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// automationRunInfo holds the most recent run data for an automation.
type automationRunInfo struct {
	Status    string
	Message   string
	StartedAt time.Time
}

// viewAutomationsContent renders the automations settings pane showing all
// registered automations with their schedule, last run status, and message.
func (p *Plugin) viewAutomationsContent(width, height int) string {
	automations := p.cfg.Automations
	if len(automations) == 0 {
		return p.styles.muted.Render("  No automations configured.\n\n  Add automations to config.yaml under the 'automations:' section.")
	}

	// Query last run info for each automation from the database.
	runMap := make(map[string]*automationRunInfo)
	if p.database != nil {
		for _, a := range automations {
			info := p.queryLastAutomationRun(a.Name)
			if info != nil {
				runMap[a.Name] = info
			}
		}
	}

	// Column widths — compute dynamically based on data.
	maxName, maxSched := 4, 8 // "NAME", "SCHEDULE"
	for _, a := range automations {
		if len(a.Name) > maxName {
			maxName = len(a.Name)
		}
		if len(a.Schedule) > maxSched {
			maxSched = len(a.Schedule)
		}
	}
	// Cap column widths to keep things readable.
	if maxName > 30 {
		maxName = 30
	}
	if maxSched > 15 {
		maxSched = 15
	}

	statusWidth := 8 // "disabled" is the longest status
	lastRunWidth := 12

	// Remaining width for message column.
	fixedWidth := 2 + maxName + 2 + maxSched + 2 + statusWidth + 2 + lastRunWidth + 2
	msgWidth := width - fixedWidth
	if msgWidth < 10 {
		msgWidth = 10
	}

	var lines []string

	// Header row.
	headerStyle := p.styles.muted
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		maxName, "NAME",
		maxSched, "SCHEDULE",
		statusWidth, "STATUS",
		lastRunWidth, "LAST RUN",
		"MESSAGE",
	)
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, "")

	// Automation rows.
	for _, a := range automations {
		name := truncate(a.Name, maxName)
		schedule := truncate(a.Schedule, maxSched)

		var status, lastRun, message string
		var statusStyle lipgloss.Style

		if !a.Enabled {
			status = "disabled"
			lastRun = "\u2014"
			message = "\u2014"
			statusStyle = p.styles.muted
		} else if info, ok := runMap[a.Name]; ok {
			status = info.Status
			lastRun = relativeTime(info.StartedAt)
			message = info.Message
			statusStyle = p.statusStyle(info.Status)
		} else {
			status = "\u2014"
			lastRun = "never"
			message = "\u2014"
			statusStyle = p.styles.muted
		}

		message = truncate(message, msgWidth)

		line := fmt.Sprintf("  %-*s  %-*s  %s  %-*s  %s",
			maxName, name,
			maxSched, schedule,
			statusStyle.Render(fmt.Sprintf("%-*s", statusWidth, status)),
			lastRunWidth, lastRun,
			message,
		)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// queryLastAutomationRun fetches the most recent run record for the named automation.
func (p *Plugin) queryLastAutomationRun(name string) *automationRunInfo {
	if p.database == nil {
		return nil
	}
	var startedAt, status, message string
	err := p.database.QueryRow(
		`SELECT started_at, status, message FROM cc_automation_runs WHERE name = ? ORDER BY started_at DESC LIMIT 1`,
		name,
	).Scan(&startedAt, &status, &message)
	if err != nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return nil
	}
	return &automationRunInfo{
		Status:    status,
		Message:   message,
		StartedAt: t,
	}
}

// statusStyle returns a lipgloss style for the given automation run status.
func (p *Plugin) statusStyle(status string) lipgloss.Style {
	switch status {
	case "success":
		return p.styles.enabled // green
	case "error":
		return p.styles.logError // red
	case "skipped":
		return p.styles.muted
	default:
		return p.styles.muted
	}
}

// relativeTime formats a timestamp as a human-readable relative time string.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

// truncate shortens a string to maxLen, adding an ellipsis if truncated.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "\u2026"
}

// truncateRunes is like truncate but operates on runes for multi-byte safety.
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "\u2026"
}

// stripAnsi removes ANSI escape sequences for width calculations.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
