// Package prs implements the PR tracking plugin for CCC.
// It shows pull requests grouped into four sub-tabs by category:
// Waiting, Respond, Review, and Stale.
package prs

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/agent"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// Compile-time check that Plugin implements plugin.Plugin.
var _ plugin.Plugin = (*Plugin)(nil)

// Plugin implements plugin.Plugin for PR tracking.
type Plugin struct {
	database    *sql.DB
	cfg         *config.Config
	styles      prsStyles
	grad        prsGrad
	logger      plugin.Logger
	bus         plugin.EventBus
	agentRunner agent.Runner
	rowStyle    prRowStyle

	prs        []db.PullRequest
	activeTab  int    // 0=waiting, 1=respond, 2=review, 3=stale
	cursors    [4]int // per-tab cursor positions
	lastLoaded time.Time
	width      int
	height     int
	frame      int

	flashMessage   string
	flashMessageAt time.Time
}

// Slug returns the plugin identifier.
func (p *Plugin) Slug() string { return "prs" }

// TabName returns the display name shown in the tab bar.
func (p *Plugin) TabName() string { return "PRs" }

// RefreshInterval returns how often the plugin should auto-refresh.
func (p *Plugin) RefreshInterval() time.Duration { return 30 * time.Second }

// Init initialises the plugin with context from the host.
func (p *Plugin) Init(ctx plugin.Context) error {
	p.database = ctx.DB
	p.cfg = ctx.Config
	p.logger = ctx.Logger
	p.bus = ctx.Bus
	p.agentRunner = ctx.AgentRunner

	if ctx.Styles != nil {
		p.styles = *ctx.Styles
	} else {
		pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
		p.styles = ui.NewStyles(pal)
	}
	if ctx.Grad != nil {
		p.grad = *ctx.Grad
	} else {
		pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
		p.grad = ui.NewGradientColors(pal)
	}
	p.rowStyle = newPRRowStyle(&p.styles)

	// Reload PR data from DB when ai-cron finishes a refresh.
	if p.bus != nil {
		p.bus.Subscribe("data.refreshed", func(e plugin.Event) {
			if p.database != nil {
				prs, _ := db.DBLoadPullRequests(p.database)
				p.prs = prs
				p.lastLoaded = time.Now()
			}
		})
	}

	return nil
}

// Shutdown cleans up plugin resources.
func (p *Plugin) Shutdown() {}

// Migrations returns DB migrations for the PR tracking plugin.
func (p *Plugin) Migrations() []plugin.Migration {
	return nil // cc_pull_requests table is created in core schema.go
}

// Routes returns navigable sub-routes.
func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "waiting", Description: "PRs waiting on reviewers"},
		{Slug: "respond", Description: "PRs needing your response"},
		{Slug: "review", Description: "PRs awaiting your review"},
		{Slug: "stale", Description: "Stale PRs with no recent activity"},
	}
}

// NavigateTo switches to the requested sub-route.
func (p *Plugin) NavigateTo(route string, args map[string]string) {
	switch route {
	case "waiting":
		p.activeTab = 0
	case "respond":
		p.activeTab = 1
	case "review":
		p.activeTab = 2
	case "stale":
		p.activeTab = 3
	}
}

// Refresh returns a tea.Cmd for refreshing PR data from DB.
func (p *Plugin) Refresh() tea.Cmd {
	database := p.database
	if database == nil {
		return nil
	}
	return func() tea.Msg {
		prs, _ := db.DBLoadPullRequests(database)
		return prsLoadedMsg{prs: prs}
	}
}

// HandleMessage processes non-key messages.
func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case prsLoadedMsg:
		p.prs = msg.prs
		p.lastLoaded = time.Now()
		// Clamp cursors to new list sizes
		for i := range p.cursors {
			filtered := p.filteredPRs(i)
			if p.cursors[i] >= len(filtered) {
				p.cursors[i] = max(0, len(filtered)-1)
			}
		}
		// Evaluate whether any PRs need an agent spawned.
		if cmd := p.evaluateAgentTriggers(); cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()

	case agent.SessionStartedMsg:
		if p.isPRAgent(msg.ID) {
			if err := db.DBUpdatePRAgentStatus(p.database, msg.ID,
				"running", "", "", "", ""); err != nil {
				p.logger.Error("prs", fmt.Sprintf("agent status update (running) failed for %s: %v", msg.ID, err))
			}
			p.updatePRAgentStatus(msg.ID, "running")
			return true, plugin.NoopAction()
		}

	case agent.SessionIDCapturedMsg:
		if p.isPRAgent(msg.ID) {
			if err := db.DBUpdatePRAgentStatus(p.database, msg.ID,
				"running", msg.SessionID, "", "", ""); err != nil {
				p.logger.Error("prs", fmt.Sprintf("agent session ID update failed for %s: %v", msg.ID, err))
			}
			p.updatePRSessionID(msg.ID, msg.SessionID)
			return true, plugin.NoopAction()
		}

	case agent.SessionFinishedMsg:
		if p.isPRAgent(msg.ID) {
			status := "completed"
			if msg.ExitCode != 0 {
				status = "failed"
			}
			var summary string
			if p.agentRunner != nil {
				if sess := p.agentRunner.Session(msg.ID); sess != nil {
					summary = agent.ExtractSessionSummary(sess)
				}
			}
			if err := db.DBUpdatePRAgentStatus(p.database, msg.ID,
				status, "", "", "", summary); err != nil {
				p.logger.Error("prs", fmt.Sprintf("agent status update (%s) failed for %s: %v", status, msg.ID, err))
			}
			p.updatePRAgentStatus(msg.ID, status)
			return true, plugin.NoopAction()
		}

	case ui.TickMsg:
		// Periodically reload PR data from DB.
		if p.database != nil && time.Since(p.lastLoaded) >= p.RefreshInterval() {
			return false, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.Refresh()}
		}
		return false, plugin.NoopAction()

	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return true, plugin.NoopAction()
	}

	return false, plugin.NoopAction()
}

// filteredPRs returns PRs matching the given tab index's category.
func (p *Plugin) filteredPRs(tabIdx int) []db.PullRequest {
	cat := categoryOrder[tabIdx]
	var out []db.PullRequest
	for _, pr := range p.prs {
		if pr.Category == cat {
			out = append(out, pr)
		}
	}
	return out
}

// categoryCounts returns the count of PRs per category.
func (p *Plugin) categoryCounts() [4]int {
	var counts [4]int
	for _, pr := range p.prs {
		for i, cat := range categoryOrder {
			if pr.Category == cat {
				counts[i]++
			}
		}
	}
	return counts
}

// resolveRepoDir finds the local directory for a GitHub repo by scanning
// learned paths' .git/config for a matching remote URL.
// repo is in "owner/repo" format (e.g. "thanx/thanx-merchant-ui").
func (p *Plugin) resolveRepoDir(repo string) string {
	if p.database == nil {
		return ""
	}
	paths, err := db.DBLoadPaths(p.database)
	if err != nil {
		return ""
	}
	for _, dir := range paths {
		gitConfig := filepath.Join(strings.TrimRight(dir, "/"), ".git", "config")
		if matchesRepo(gitConfig, repo) {
			return strings.TrimRight(dir, "/")
		}
	}
	return ""
}

// isPRAgent returns true if the given agent ID matches a PR in our list.
// PR agent IDs are PR IDs like "owner/repo#123".
func (p *Plugin) isPRAgent(id string) bool {
	for _, pr := range p.prs {
		if pr.ID == id {
			return true
		}
	}
	return false
}

// updatePRAgentStatus updates the in-memory agent status for a PR.
func (p *Plugin) updatePRAgentStatus(id, status string) {
	for i := range p.prs {
		if p.prs[i].ID == id {
			p.prs[i].AgentStatus = status
			return
		}
	}
}

// updatePRSessionID updates the in-memory agent session ID for a PR.
func (p *Plugin) updatePRSessionID(id, sessionID string) {
	for i := range p.prs {
		if p.prs[i].ID == id {
			p.prs[i].AgentSessionID = sessionID
			return
		}
	}
}

// matchesRepo checks if a .git/config file contains a remote URL matching
// the given "owner/repo" string.
func matchesRepo(gitConfigPath, repo string) bool {
	f, err := os.Open(gitConfigPath)
	if err != nil {
		return false
	}
	defer f.Close()

	// Match against github.com:owner/repo or github.com/owner/repo
	sshSuffix := "github.com:" + repo
	httpsSuffix := "github.com/" + repo

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "url =") {
			continue
		}
		url := strings.TrimSpace(strings.TrimPrefix(line, "url ="))
		url = strings.TrimSuffix(url, ".git")
		if strings.HasSuffix(url, sshSuffix) || strings.HasSuffix(url, httpsSuffix) {
			return true
		}
	}
	return false
}
