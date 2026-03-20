package prs

// PR category constants matching db.PullRequest.Category values.
const (
	CategoryWaiting = "waiting"
	CategoryRespond = "respond"
	CategoryReview  = "review"
	CategoryStale   = "stale"
)

// categoryOrder defines the tab index for each category.
var categoryOrder = [4]string{
	CategoryWaiting,
	CategoryRespond,
	CategoryReview,
	CategoryStale,
}

// categoryDisplayName maps category slugs to display names.
var categoryDisplayName = map[string]string{
	CategoryWaiting: "Waiting",
	CategoryRespond: "Respond",
	CategoryReview:  "Review",
	CategoryStale:   "Stale",
}

// categoryEmptyMessage maps category slugs to empty-state messages.
var categoryEmptyMessage = map[string]string{
	CategoryWaiting: "No PRs waiting on reviewers -- you're all clear!",
	CategoryRespond: "No PRs need your response right now.",
	CategoryReview:  "No PRs waiting for your review.",
	CategoryStale:   "No stale PRs -- everything is moving.",
}
