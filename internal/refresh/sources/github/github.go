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

// GitHubSource fetches open PRs authored by the user from configured repos.
type GitHubSource struct {
	Repos    []string
	Username string
	enabled  bool
}

// New creates a GitHubSource with the given config.
func New(enabled bool, repos []string, username string) *GitHubSource {
	return &GitHubSource{
		Repos:    repos,
		Username: username,
		enabled:  enabled,
	}
}

func (s *GitHubSource) Name() string  { return "github" }
func (s *GitHubSource) Enabled() bool { return s.enabled }

func (s *GitHubSource) Fetch(ctx context.Context) (*refresh.SourceResult, error) {
	threads, err := fetchGitHubThreads(ctx, s.Repos)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	return &refresh.SourceResult{Threads: threads}, nil
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
