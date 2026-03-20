package github

import (
	"context"
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
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
	username := s.Username
	if username == "" {
		login, err := fetchGHUsername()
		if err != nil {
			return nil, fmt.Errorf("github: could not determine username (set github.username in config or run 'gh auth login'): %w", err)
		}
		username = login
	}

	var warnings []db.Warning

	authored, err := fetchAuthoredPRs(ctx)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}

	requested, err := fetchReviewRequestedPRs(ctx)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}

	// Collect unique PRs for detail fetching.
	type prRef struct {
		repo   string
		number int
	}
	seen := make(map[string]bool)
	var refs []prRef
	for _, pr := range authored {
		key := prKey(pr.Repository.NameWithOwner, pr.Number)
		if !seen[key] {
			seen[key] = true
			refs = append(refs, prRef{pr.Repository.NameWithOwner, pr.Number})
		}
	}
	for _, pr := range requested {
		key := prKey(pr.Repository.NameWithOwner, pr.Number)
		if !seen[key] {
			seen[key] = true
			refs = append(refs, prRef{pr.Repository.NameWithOwner, pr.Number})
		}
	}

	// Fetch detail for each PR. Individual failures produce warnings, not errors.
	details := make(map[string]*PRDetail, len(refs))
	for _, ref := range refs {
		detail, err := fetchPRDetail(ctx, ref.repo, ref.number)
		if err != nil {
			warnings = append(warnings, db.Warning{
				Source:  "github",
				Message: fmt.Sprintf("failed to fetch detail for %s#%d: %v", ref.repo, ref.number, err),
				At:      time.Now(),
			})
			continue
		}
		details[prKey(ref.repo, ref.number)] = detail
	}

	prs := buildPullRequests(authored, requested, details, username, time.Now())

	return &refresh.SourceResult{
		PullRequests: prs,
		Warnings:     warnings,
	}, nil
}
