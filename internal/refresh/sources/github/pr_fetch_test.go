package github

import (
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestComputeCategory_Review(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "reviewer",
		PendingReviewerLogins: []string{"alice"},
		ReviewDecision:        "",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "review" {
		t.Errorf("expected 'review', got %q", got)
	}
}

func TestComputeCategory_ReviewBothRole(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "both",
		PendingReviewerLogins: []string{"bob"},
		ReviewDecision:        "",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "bob", now)
	if got != "review" {
		t.Errorf("expected 'review', got %q", got)
	}
}

func TestComputeCategory_ReviewNotPending(t *testing.T) {
	// Reviewer role but NOT in pending list -> should not be "review".
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "reviewer",
		PendingReviewerLogins: []string{"someone-else"},
		ReviewDecision:        "",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "waiting" {
		t.Errorf("expected 'waiting', got %q", got)
	}
}

func TestComputeCategory_Respond(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "author",
		PendingReviewerLogins: nil,
		ReviewDecision:        "CHANGES_REQUESTED",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "respond" {
		t.Errorf("expected 'respond', got %q", got)
	}
}

func TestComputeCategory_RespondBothRole(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "both",
		PendingReviewerLogins: nil, // not in pending list, so "review" won't match
		ReviewDecision:        "CHANGES_REQUESTED",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "respond" {
		t.Errorf("expected 'respond', got %q", got)
	}
}

func TestComputeCategory_Stale(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "author",
		PendingReviewerLogins: nil,
		ReviewDecision:        "",
		LastActivityAt:        now.Add(-15 * 24 * time.Hour), // 15 days ago
	}
	got := computeCategory(pr, "alice", now)
	if got != "stale" {
		t.Errorf("expected 'stale', got %q", got)
	}
}

func TestComputeCategory_Waiting(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "author",
		PendingReviewerLogins: nil,
		ReviewDecision:        "REVIEW_REQUIRED",
		LastActivityAt:        now.Add(-1 * time.Hour), // recent
	}
	got := computeCategory(pr, "alice", now)
	if got != "waiting" {
		t.Errorf("expected 'waiting', got %q", got)
	}
}

func TestComputeCategory_ReviewTakesPrecedenceOverRespond(t *testing.T) {
	// Both author and reviewer, pending review AND changes requested.
	// "review" should win since it's checked first.
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "both",
		PendingReviewerLogins: []string{"alice"},
		ReviewDecision:        "CHANGES_REQUESTED",
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "review" {
		t.Errorf("expected 'review', got %q", got)
	}
}

func TestComputeCategory_CaseInsensitiveUsername(t *testing.T) {
	now := time.Now()
	pr := db.PullRequest{
		MyRole:                "reviewer",
		PendingReviewerLogins: []string{"Alice"},
		LastActivityAt:        now,
	}
	got := computeCategory(pr, "alice", now)
	if got != "review" {
		t.Errorf("expected 'review' with case-insensitive match, got %q", got)
	}
}

func TestBuildPullRequests_Dedup(t *testing.T) {
	now := time.Now()
	authored := []RawPR{
		{
			Number: 42,
			Title:  "Fix bug",
			URL:    "https://github.com/owner/repo/pull/42",
			IsDraft: false,
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
	}
	authored[0].Repository.NameWithOwner = "owner/repo"
	authored[0].Author.Login = "alice"

	// Same PR appears in review-requested.
	requested := []RawPR{
		{
			Number: 42,
			Title:  "Fix bug",
			URL:    "https://github.com/owner/repo/pull/42",
			IsDraft: false,
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
	}
	requested[0].Repository.NameWithOwner = "owner/repo"
	requested[0].Author.Login = "alice"

	details := map[string]*PRDetail{
		"owner/repo#42": {
			ReviewDecision: "REVIEW_REQUIRED",
			ReviewRequests: []struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			}{
				{Login: "alice"},
			},
		},
	}

	prs := buildPullRequests(authored, requested, details, "alice", now)
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR after dedup, got %d", len(prs))
	}
	if prs[0].MyRole != "both" {
		t.Errorf("expected role 'both', got %q", prs[0].MyRole)
	}
	if prs[0].ID != "owner/repo#42" {
		t.Errorf("expected ID 'owner/repo#42', got %q", prs[0].ID)
	}
}

func TestBuildPullRequests_SeparatePRs(t *testing.T) {
	now := time.Now()
	authored := []RawPR{
		{Number: 1, Title: "My PR"},
	}
	authored[0].Repository.NameWithOwner = "owner/repo"
	authored[0].Author.Login = "alice"
	authored[0].UpdatedAt = now

	requested := []RawPR{
		{Number: 2, Title: "Review me"},
	}
	requested[0].Repository.NameWithOwner = "owner/repo"
	requested[0].Author.Login = "bob"
	requested[0].UpdatedAt = now

	prs := buildPullRequests(authored, requested, nil, "alice", now)
	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	roles := map[string]bool{}
	for _, pr := range prs {
		roles[pr.MyRole] = true
	}
	if !roles["author"] || !roles["reviewer"] {
		t.Errorf("expected author and reviewer roles, got %v", roles)
	}
}

func TestBuildPullRequests_NilDetailGraceful(t *testing.T) {
	now := time.Now()
	authored := []RawPR{
		{Number: 10, Title: "No detail"},
	}
	authored[0].Repository.NameWithOwner = "owner/repo"
	authored[0].Author.Login = "alice"
	authored[0].UpdatedAt = now

	// No details available (detail fetch failed).
	prs := buildPullRequests(authored, nil, nil, "alice", now)
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].ReviewDecision != "" {
		t.Errorf("expected empty review decision, got %q", prs[0].ReviewDecision)
	}
	if prs[0].Category != "waiting" {
		t.Errorf("expected 'waiting' category, got %q", prs[0].Category)
	}
}

func TestComputeRole(t *testing.T) {
	tests := []struct {
		author, requested bool
		want              string
	}{
		{true, false, "author"},
		{false, true, "reviewer"},
		{true, true, "both"},
	}
	for _, tt := range tests {
		got := computeRole(tt.author, tt.requested)
		if got != tt.want {
			t.Errorf("computeRole(%v, %v) = %q, want %q", tt.author, tt.requested, got, tt.want)
		}
	}
}

func TestComputeCIStatus(t *testing.T) {
	type check = struct {
		State      string `json:"state"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	tests := []struct {
		name   string
		checks []check
		want   string
	}{
		{"empty", nil, ""},
		{"all success", []check{{State: "SUCCESS"}}, "success"},
		{"one failure", []check{{State: "SUCCESS"}, {State: "FAILURE"}}, "failure"},
		{"pending", []check{{State: "SUCCESS"}, {State: "PENDING"}}, "pending"},
		{"conclusion failure", []check{{Conclusion: "FAILURE"}}, "failure"},
		{"in progress", []check{{Status: "IN_PROGRESS"}}, "pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeCIStatus(tt.checks)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPullRequests_PopulatesHeadSHA(t *testing.T) {
	now := time.Now()
	authored := []RawPR{{
		Number: 1, Title: "Test", URL: "https://github.com/t/r/pull/1",
		Repository: struct{ NameWithOwner string `json:"nameWithOwner"` }{NameWithOwner: "t/r"},
		Author:     struct{ Login string `json:"login"` }{Login: "alice"},
		CreatedAt:  now, UpdatedAt: now,
	}}
	details := map[string]*PRDetail{
		"t/r#1": {HeadRefOid: "abc123"},
	}
	prs := buildPullRequests(authored, nil, details, "alice", now)
	if len(prs) != 1 {
		t.Fatal("expected 1 PR")
	}
	if prs[0].HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123", prs[0].HeadSHA)
	}
}

func TestExtractPendingReviewerLogins(t *testing.T) {
	detail := &PRDetail{
		ReviewRequests: []struct {
			Login string `json:"login"`
			Name  string `json:"name"`
		}{
			{Login: "alice"},
			{Login: "bob"},
			{Name: "team-foo"}, // team request, no login
		},
	}
	logins := extractPendingReviewerLogins(detail, "me")
	if len(logins) != 2 {
		t.Fatalf("expected 2 logins, got %d: %v", len(logins), logins)
	}
	if logins[0] != "alice" || logins[1] != "bob" {
		t.Errorf("expected [alice bob], got %v", logins)
	}
}
