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
	case "o":
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

	// Launch PR review, resume agent session, or navigate to running agent
	case "enter":
		filtered := p.filteredPRs(p.activeTab)
		if len(filtered) == 0 {
			return plugin.ConsumedAction()
		}
		pr := filtered[p.cursors[p.activeTab]]

		switch pr.AgentStatus {
		case "completed", "failed":
			// Resume the agent's finished session for inspection
			if pr.AgentSessionID != "" {
				dir := p.resolveRepoDir(pr.Repo)
				if dir == "" {
					return plugin.ConsumedAction()
				}
				return plugin.Action{
					Type: plugin.ActionLaunch,
					Args: map[string]string{
						"dir":       dir,
						"resume_id": pr.AgentSessionID,
					},
				}
			}
		case "running":
			// Navigate to the command center which shows running agents
			return plugin.Action{Type: plugin.ActionNavigate, Payload: "command"}
		case "pending":
			return plugin.ConsumedAction()
		}

		// No agent — launch manual review/respond session
		dir := p.resolveRepoDir(pr.Repo)
		if dir == "" {
			// Can't find local repo — fall back to opening in browser
			if pr.URL != "" {
				return plugin.Action{Type: plugin.ActionOpenURL, Payload: pr.URL}
			}
			return plugin.ConsumedAction()
		}
		prompt := fmt.Sprintf("/pr-review-toolkit:review-pr %s", pr.URL)
		if pr.Category == CategoryRespond {
			prompt = fmt.Sprintf("/pr-respond %s", pr.URL)
		}
		return plugin.Action{
			Type: plugin.ActionLaunch,
			Args: map[string]string{
				"dir":            dir,
				"initial_prompt": prompt,
			},
		}

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
		{Key: "enter", Description: "Review/respond (resume agent or launch)", Promoted: true},
		{Key: "o", Description: "Open PR in browser", Promoted: true},
		{Key: "r", Description: "Refresh", Promoted: true},
	}
}
