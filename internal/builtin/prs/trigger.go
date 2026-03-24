package prs

import (
	"fmt"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/daemon"
	"github.com/anutron/claude-command-center/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

// needsAgent returns true if the given PR should trigger an agent run.
// It is a pure function with no side effects.
func needsAgent(pr db.PullRequest) bool {
	// Only review and respond categories trigger agents.
	if pr.Category != CategoryReview && pr.Category != CategoryRespond {
		return false
	}
	// Don't interrupt an already running or pending agent.
	if pr.AgentStatus == "running" || pr.AgentStatus == "pending" {
		return false
	}
	// Never run before — trigger.
	if pr.AgentHeadSHA == "" {
		return true
	}
	// New commits pushed since last agent run.
	if pr.HeadSHA != pr.AgentHeadSHA {
		return true
	}
	// Category changed (e.g. review → respond).
	if pr.Category != pr.AgentCategory {
		return true
	}
	return false
}

// evaluateAgentTriggers scans loaded PRs and spawns agents for any that
// qualify. Prefers the daemon RPC for agent operations; falls back to the
// local agentRunner if the daemon is not connected.
func (p *Plugin) evaluateAgentTriggers() tea.Cmd {
	client := p.getDaemonClient()
	if client == nil && p.agentRunner == nil {
		return nil
	}

	var cmds []tea.Cmd
	for _, pr := range p.prs {
		if !needsAgent(pr) {
			continue
		}
		dir := p.resolveRepoDir(pr.Repo)
		if dir == "" {
			continue
		}
		isRespond := pr.Category == CategoryRespond
		prompt := fmt.Sprintf("/pr-review-toolkit:review-pr %s", pr.URL)
		if isRespond {
			prompt = fmt.Sprintf("/pr-respond --apply %s", pr.URL)
		}

		prCopy := pr

		// Try daemon RPC first, fall back to local runner.
		if client != nil {
			_ = client.LaunchAgent(daemon.LaunchAgentParams{
				ID:         prCopy.ID,
				Prompt:     prompt,
				Dir:        dir,
				Worktree:   isRespond,
				Permission: "default",
				Automation: "pr-review",
			})
		} else if p.agentRunner != nil {
			req := agent.Request{
				ID:         prCopy.ID,
				Prompt:     prompt,
				ProjectDir: dir,
				Worktree:   isRespond,
				Permission: "default",
				Automation: "pr-review",
			}
			_, cmd := p.agentRunner.LaunchOrQueue(req)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Mark as pending immediately so we don't re-trigger on the next tick.
		_ = db.DBUpdatePRAgentStatus(p.database, prCopy.ID,
			"pending", "", prCopy.Category, prCopy.HeadSHA, "")
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}
