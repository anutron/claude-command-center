package github

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ContextFetcherImpl implements refresh.ContextFetcher for GitHub PRs and issues.
type ContextFetcherImpl struct{}

// NewContextFetcher creates a new GitHub ContextFetcher.
func NewContextFetcher() *ContextFetcherImpl {
	return &ContextFetcherImpl{}
}

func (f *ContextFetcherImpl) ContextTTL() time.Duration { return 24 * time.Hour }

func (f *ContextFetcherImpl) FetchContext(sourceRef string) (string, error) {
	out, err := exec.Command("gh", "pr", "view", sourceRef, "--json",
		"title,body,comments,reviews", "--jq",
		`"# " + .title + "\n\n" + .body + "\n\n## Comments\n" + ([.comments[] | "**" + .author.login + ":** " + .body] | join("\n\n")) + "\n\n## Reviews\n" + ([.reviews[] | "**" + .author.login + " (" + .state + "):** " + .body] | join("\n\n"))`).Output()
	if err != nil {
		out, err = exec.Command("gh", "issue", "view", sourceRef, "--json",
			"title,body,comments", "--jq",
			`"# " + .title + "\n\n" + .body + "\n\n## Comments\n" + ([.comments[] | "**" + .author.login + ":** " + .body] | join("\n\n"))`).Output()
		if err != nil {
			return "", fmt.Errorf("gh view %s: %w", sourceRef, err)
		}
	}
	return strings.TrimSpace(string(out)), nil
}
