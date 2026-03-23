package settings

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/charmbracelet/huh"
)

// buildPRSettingsForm creates a form showing ignored repos and ignored PRs.
func (p *Plugin) buildPRSettingsForm() *huh.Form {
	// Load ignored repos
	repos, _ := db.DBLoadIgnoredRepos(p.database)
	var repoLines string
	if len(repos) == 0 {
		repoLines = p.styles.muted.Render("No repos ignored. Press I on a PR to ignore its repo.")
	} else {
		var lines []string
		for _, r := range repos {
			lines = append(lines, fmt.Sprintf("  • %s", r))
		}
		repoLines = strings.Join(lines, "\n")
	}

	// Load ignored PRs
	ignoredPRs, _ := db.DBLoadIgnoredPRs(p.database)
	var prLines string
	if len(ignoredPRs) == 0 {
		prLines = p.styles.muted.Render("No PRs ignored. Press i on a PR to ignore it.")
	} else {
		var lines []string
		for _, pr := range ignoredPRs {
			lines = append(lines, fmt.Sprintf("  • %s  %s", pr.ID, p.styles.muted.Render(pr.Title)))
		}
		prLines = strings.Join(lines, "\n")
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Ignored Repos").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("PRs from these repos are hidden from all tabs:"),
					repoLines,
				)),
			huh.NewNote().
				Title("Ignored PRs").
				Description(fmt.Sprintf(
					"%s\n%s",
					p.styles.muted.Render("These individual PRs are hidden (auto-cleaned when closed):"),
					prLines,
				)),
		),
	).WithShowHelp(false).WithShowErrors(false).WithTheme(p.styles.huhTheme)

	return form
}
