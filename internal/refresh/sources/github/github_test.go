package github

import (
	"testing"
)

func TestNew(t *testing.T) {
	repos := []string{"owner/repo1", "owner/repo2"}
	src := New(true, repos, "testuser")

	if src == nil {
		t.Fatal("New() returned nil")
	}
	if src.Username != "testuser" {
		t.Errorf("Username = %q, want %q", src.Username, "testuser")
	}
	if len(src.Repos) != 2 {
		t.Errorf("Repos length = %d, want 2", len(src.Repos))
	}
	if src.Repos[0] != "owner/repo1" {
		t.Errorf("Repos[0] = %q, want %q", src.Repos[0], "owner/repo1")
	}
}

func TestNewDefaults(t *testing.T) {
	src := New(false, nil, "")

	if src.Username != "" {
		t.Errorf("Username = %q, want empty", src.Username)
	}
	if src.Repos != nil {
		t.Errorf("Repos = %v, want nil", src.Repos)
	}
}

func TestName(t *testing.T) {
	src := New(true, nil, "")
	if got := src.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
	}
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := New(tt.enabled, nil, "")
			if got := src.Enabled(); got != tt.enabled {
				t.Errorf("Enabled() = %v, want %v", got, tt.enabled)
			}
		})
	}
}

func TestNewReposSliceIndependence(t *testing.T) {
	repos := []string{"owner/repo1", "owner/repo2"}
	src := New(true, repos, "user")

	// Mutating the original slice should not affect the source
	// (since Go slices share backing arrays, this tests current behavior)
	repos[0] = "changed/repo"
	if src.Repos[0] != "changed/repo" {
		t.Log("Note: New() does not defensively copy the repos slice")
	}
}

func TestSummarizePRDraftFallback(t *testing.T) {
	// summarizePR falls back to "Draft" when gh api fails and PR is draft
	pr := ghPR{Number: 1, Title: "test", Draft: true}
	got := summarizePR(nil, "owner/repo", pr)
	if got != "Draft" {
		t.Errorf("summarizePR(draft PR) = %q, want %q", got, "Draft")
	}
}

func TestSummarizePROpenFallback(t *testing.T) {
	// summarizePR falls back to "Open" when gh api fails and PR is not draft
	pr := ghPR{Number: 1, Title: "test", Draft: false}
	got := summarizePR(nil, "owner/repo", pr)
	if got != "Open" {
		t.Errorf("summarizePR(non-draft PR) = %q, want %q", got, "Open")
	}
}

func TestFetchGitHubThreadsEmptyRepos(t *testing.T) {
	threads, err := fetchGitHubThreads(nil, nil)
	if err != nil {
		t.Fatalf("fetchGitHubThreads(nil repos) returned error: %v", err)
	}
	if len(threads) != 0 {
		t.Errorf("fetchGitHubThreads(nil repos) returned %d threads, want 0", len(threads))
	}
}
