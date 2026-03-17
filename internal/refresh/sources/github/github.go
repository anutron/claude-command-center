package github

import (
	"context"

	"github.com/anutron/claude-command-center/internal/refresh"
)

// GitHubSource fetches open PRs authored by the user from configured repos.
// NOTE: The threads feature has been removed. This source currently returns
// an empty result. It remains as a placeholder for future GitHub integration
// (e.g., PR review todos).
type GitHubSource struct {
	Repos      []string
	Username   string
	TrackMyPRs bool
	enabled    bool
}

// New creates a GitHubSource with the given config.
func New(enabled bool, repos []string, username string, trackMyPRs bool) *GitHubSource {
	return &GitHubSource{
		Repos:      repos,
		Username:   username,
		TrackMyPRs: trackMyPRs,
		enabled:    enabled,
	}
}

func (s *GitHubSource) Name() string  { return "github" }
func (s *GitHubSource) Enabled() bool { return s.enabled }

func (s *GitHubSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	// The threads feature has been removed. GitHub PRs were only surfaced as
	// threads, so this source now returns an empty result.
	return &refresh.SourceResult{}, nil
}
