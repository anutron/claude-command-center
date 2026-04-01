package ui

import (
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/lipgloss"
)

// AgentStatusIcon returns a single-character icon for the given agent status.
func AgentStatusIcon(status string) string {
	switch status {
	case "running", "processing":
		return "●"
	case "queued":
		return "◌"
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "stopped":
		return "⊘"
	case "blocked":
		return "⏸"
	default:
		return "?"
	}
}

// AgentStatusColor returns the lipgloss color for the given agent status.
func AgentStatusColor(status string) lipgloss.Color {
	switch status {
	case "running", "processing":
		return lipgloss.Color("#4ade80")
	case "queued":
		return lipgloss.Color("#f59e0b")
	case "completed":
		return lipgloss.Color("#565f89")
	case "failed", "stopped":
		return lipgloss.Color("#f7768e")
	case "blocked":
		return lipgloss.Color("#f59e0b")
	default:
		return lipgloss.Color("#565f89")
	}
}

// FormatAgentElapsed returns a human-readable elapsed time for the entry.
func FormatAgentElapsed(e db.AgentHistoryEntry) string {
	if e.Status == "queued" {
		return "queued"
	}
	if e.DurationSec > 0 {
		return FormatDuration(time.Duration(e.DurationSec) * time.Second)
	}
	if !e.StartedAt.IsZero() {
		return FormatDuration(time.Since(e.StartedAt))
	}
	return "—"
}

// FormatDuration formats a duration as a short human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}
