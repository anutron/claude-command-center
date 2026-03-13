package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/refresh"
)

// GitHubSource fetches open PRs authored by the user from configured repos,
// and optionally all PRs requiring the user's attention across all repos.
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
	var allThreads []db.Thread

	// Fetch PRs from explicitly configured repos (authored by user).
	if len(s.Repos) > 0 {
		repoThreads, err := fetchGitHubThreads(ctx, s.Repos)
		if err != nil {
			return nil, fmt.Errorf("repo fetch failed: %w", err)
		}
		allThreads = append(allThreads, repoThreads...)
	}

	// Fetch all PRs requiring the user's attention across all repos.
	if s.TrackMyPRs {
		myThreads, err := searchMyPRs(ctx)
		if err != nil {
			// Non-fatal: log but continue with repo-specific results.
			_ = err
		} else {
			allThreads = append(allThreads, myThreads...)
		}
	}

	// Deduplicate by URL.
	allThreads = deduplicateThreads(allThreads)

	return &refresh.SourceResult{Threads: allThreads}, nil
}

type ghPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	URL       string `json:"html_url"`
	State     string `json:"state"`
	Draft     bool   `json:"draft"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type ghComment struct {
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Path     string `json:"path"`
	Position int    `json:"position"`
}

func fetchGitHubThreads(ctx context.Context, repos []string) ([]db.Thread, error) {
	var threads []db.Thread

	for _, repo := range repos {
		prs, err := listMyPRs(ctx, repo)
		if err != nil {
			continue
		}

		for _, pr := range prs {
			parts := strings.SplitN(repo, "/", 2)
			if len(parts) != 2 {
				continue
			}

			summary := summarizePR(ctx, repo, pr)

			threads = append(threads, db.Thread{
				Type:    "pr",
				Title:   fmt.Sprintf("#%d %s", pr.Number, pr.Title),
				URL:     pr.URL,
				Repo:    repo,
				Summary: summary,
			})
		}
	}

	return threads, nil
}

func listMyPRs(_ context.Context, repo string) ([]ghPR, error) {
	out, err := exec.Command("gh", "pr", "list",
		"--repo", repo,
		"--author", "@me",
		"--state", "open",
		"--json", "number,title,url,isDraft",
		"--limit", "20",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list for %s: %w", repo, err)
	}

	var items []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		IsDraft bool   `json:"isDraft"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}

	var prs []ghPR
	for _, item := range items {
		prs = append(prs, ghPR{
			Number: item.Number,
			Title:  item.Title,
			URL:    item.URL,
			Draft:  item.IsDraft,
		})
	}
	return prs, nil
}

// ghSearchPR represents a PR returned by `gh search prs`.
type ghSearchPR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      string `json:"state"`
	IsDraft    bool   `json:"isDraft"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// searchMyPRsFn is a variable for testability.
var searchMyPRsFn = func(ctx context.Context) ([]db.Thread, error) {
	var allPRs []ghSearchPR

	// Search for PRs where review is requested from the user.
	reviewPRs, err := ghSearchPRs("--review-requested=@me")
	if err == nil {
		allPRs = append(allPRs, reviewPRs...)
	}

	// Search for PRs assigned to the user.
	assignedPRs, err := ghSearchPRs("--assignee=@me")
	if err == nil {
		allPRs = append(allPRs, assignedPRs...)
	}

	// Search for PRs authored by the user.
	authoredPRs, err := ghSearchPRs("--author=@me")
	if err == nil {
		allPRs = append(allPRs, authoredPRs...)
	}

	// Deduplicate by URL.
	seen := make(map[string]bool)
	var threads []db.Thread
	for _, pr := range allPRs {
		if seen[pr.URL] {
			continue
		}
		seen[pr.URL] = true

		repo := pr.Repository.NameWithOwner
		summary := "Open"
		if pr.IsDraft {
			summary = "Draft"
		}

		threads = append(threads, db.Thread{
			Type:    "pr",
			Title:   fmt.Sprintf("#%d %s", pr.Number, pr.Title),
			URL:     pr.URL,
			Repo:    repo,
			Summary: summary,
		})
	}

	return threads, nil
}

func searchMyPRs(ctx context.Context) ([]db.Thread, error) {
	return searchMyPRsFn(ctx)
}

// ghSearchPRs runs `gh search prs` with the given filter flag and returns results.
func ghSearchPRs(filter string) ([]ghSearchPR, error) {
	out, err := exec.Command("gh", "search", "prs",
		filter,
		"--state=open",
		"--json", "number,title,url,isDraft,repository",
		"--limit", "50",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh search prs %s: %w", filter, err)
	}

	var prs []ghSearchPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// deduplicateThreads removes duplicate threads by URL, keeping the first occurrence.
func deduplicateThreads(threads []db.Thread) []db.Thread {
	seen := make(map[string]bool, len(threads))
	out := make([]db.Thread, 0, len(threads))
	for _, t := range threads {
		if t.URL != "" && seen[t.URL] {
			continue
		}
		if t.URL != "" {
			seen[t.URL] = true
		}
		out = append(out, t)
	}
	return out
}

func summarizePR(_ context.Context, repo string, pr ghPR) string {
	out, err := exec.Command("gh", "api",
		fmt.Sprintf("/repos/%s/pulls/%d/comments", repo, pr.Number),
	).Output()
	if err != nil {
		if pr.Draft {
			return "Draft"
		}
		return "Open"
	}

	var comments []ghComment
	if err := json.Unmarshal(out, &comments); err != nil || len(comments) == 0 {
		if pr.Draft {
			return "Draft"
		}
		return "Open, no comments"
	}

	return fmt.Sprintf("%d review comments", len(comments))
}
