package prs

import (
	"testing"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestNeedsAgent(t *testing.T) {
	tests := []struct {
		name string
		pr   db.PullRequest
		want bool
	}{
		{
			"new review PR",
			db.PullRequest{Category: CategoryReview, HeadSHA: "abc", AgentHeadSHA: "", AgentStatus: ""},
			true,
		},
		{
			"new respond PR",
			db.PullRequest{Category: CategoryRespond, HeadSHA: "abc", AgentHeadSHA: "", AgentStatus: ""},
			true,
		},
		{
			"waiting PR",
			db.PullRequest{Category: CategoryWaiting, HeadSHA: "abc"},
			false,
		},
		{
			"stale PR",
			db.PullRequest{Category: CategoryStale, HeadSHA: "abc"},
			false,
		},
		{
			"same SHA same category — no retrigger",
			db.PullRequest{Category: CategoryReview, HeadSHA: "abc", AgentHeadSHA: "abc", AgentCategory: CategoryReview, AgentStatus: "completed"},
			false,
		},
		{
			"new SHA triggers agent",
			db.PullRequest{Category: CategoryReview, HeadSHA: "def", AgentHeadSHA: "abc", AgentCategory: CategoryReview, AgentStatus: "completed"},
			true,
		},
		{
			"category changed triggers agent",
			db.PullRequest{Category: CategoryRespond, HeadSHA: "abc", AgentHeadSHA: "abc", AgentCategory: CategoryReview, AgentStatus: "completed"},
			true,
		},
		{
			"already running — no retrigger",
			db.PullRequest{Category: CategoryReview, HeadSHA: "def", AgentHeadSHA: "abc", AgentStatus: "running"},
			false,
		},
		{
			"already pending — no retrigger",
			db.PullRequest{Category: CategoryReview, HeadSHA: "def", AgentHeadSHA: "abc", AgentStatus: "pending"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsAgent(tt.pr); got != tt.want {
				t.Errorf("needsAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}
