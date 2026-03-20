package prs

import (
	"fmt"
	"os/exec"

	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

// HandleKey processes key input and returns an action for the host.
func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	// Direct sub-tab switching
	case "1":
		p.activeTab = 0
		return plugin.ConsumedAction()
	case "2":
		p.activeTab = 1
		return plugin.ConsumedAction()
	case "3":
		p.activeTab = 2
		return plugin.ConsumedAction()
	case "4":
		p.activeTab = 3
		return plugin.ConsumedAction()

	// Cycle sub-tabs
	case "right", "l":
		p.activeTab = (p.activeTab + 1) % 4
		return plugin.ConsumedAction()
	case "left", "h":
		p.activeTab = (p.activeTab + 3) % 4
		return plugin.ConsumedAction()

	// Move cursor within filtered list
	case "down", "j":
		filtered := p.filteredPRs(p.activeTab)
		if len(filtered) > 0 {
			if p.cursors[p.activeTab] < len(filtered)-1 {
				p.cursors[p.activeTab]++
			} else {
				p.cursors[p.activeTab] = 0
			}
		}
		return plugin.ConsumedAction()
	case "up", "k":
		filtered := p.filteredPRs(p.activeTab)
		if len(filtered) > 0 {
			if p.cursors[p.activeTab] > 0 {
				p.cursors[p.activeTab]--
			} else {
				p.cursors[p.activeTab] = len(filtered) - 1
			}
		}
		return plugin.ConsumedAction()

	// Open PR in browser
	case "enter", "o":
		filtered := p.filteredPRs(p.activeTab)
		if len(filtered) == 0 {
			return plugin.ConsumedAction()
		}
		pr := filtered[p.cursors[p.activeTab]]
		if pr.URL != "" {
			return plugin.Action{Type: plugin.ActionOpenURL, Payload: pr.URL}
		}
		// Fallback: use gh CLI
		cmd := exec.Command("gh", "pr", "view", "--web", "-R", pr.Repo, fmt.Sprintf("%d", pr.Number))
		_ = cmd.Start()
		return plugin.ConsumedAction()

	// Force refresh
	case "r":
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.Refresh()}
	}

	return plugin.NoopAction()
}

// KeyBindings returns the key bindings for this plugin.
func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	return []plugin.KeyBinding{
		{Key: "1/2/3/4", Description: "Switch sub-tab", Promoted: true},
		{Key: "<-/->", Description: "Cycle sub-tabs", Promoted: true},
		{Key: "j/k", Description: "Navigate PRs", Promoted: true},
		{Key: "enter/o", Description: "Open PR in browser", Promoted: true},
		{Key: "r", Description: "Refresh", Promoted: true},
	}
}
