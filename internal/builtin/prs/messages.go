package prs

import "github.com/anutron/claude-command-center/internal/db"

// prsLoadedMsg is sent when PR data is loaded from the database.
type prsLoadedMsg struct {
	prs []db.PullRequest
}
