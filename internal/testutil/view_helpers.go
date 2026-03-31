// Package testutil provides shared test infrastructure for view-level tests.
package testutil

import (
	"strings"
	"testing"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// AssertViewContains fails with a full view dump if text is not found.
func AssertViewContains(t *testing.T, view, text string) {
	t.Helper()
	if !strings.Contains(view, text) {
		t.Errorf("expected view to contain %q\n\nView:\n%s", text, view)
	}
}

// AssertViewNotContains fails with a full view dump if text is found.
func AssertViewNotContains(t *testing.T, view, text string) {
	t.Helper()
	if strings.Contains(view, text) {
		t.Errorf("expected view NOT to contain %q\n\nView:\n%s", text, view)
	}
}

// KeyMsg creates a tea.KeyMsg for a rune string (e.g., "a", "A", "/").
func KeyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// SpecialKeyMsg creates a tea.KeyMsg for a special key (e.g., tea.KeyEnter).
func SpecialKeyMsg(k tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: k}
}

// SendKeys sends a sequence of key messages to a plugin via HandleKey.
func SendKeys(p plugin.Plugin, keys ...tea.KeyMsg) {
	for _, k := range keys {
		p.HandleKey(k)
	}
}
