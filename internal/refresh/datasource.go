package refresh

import (
	"context"
	"database/sql"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/sanitize"
)

// DataSource is an extensible interface for the refresh pipeline.
// Each implementation owns its auth loading, enablement logic, and data fetching.
type DataSource interface {
	// Name returns a stable identifier used for logging and warning attribution.
	Name() string
	// Enabled returns whether this source should be fetched.
	Enabled() bool
	// Fetch loads auth, fetches data, and returns results.
	Fetch(ctx context.Context) (*SourceResult, error)
}

// PostMerger is optionally implemented by DataSources that need to perform
// actions after the merge step (e.g., calendar pending action execution).
type PostMerger interface {
	PostMerge(ctx context.Context, db *sql.DB, cc *db.CommandCenter, verbose bool) error
}

// SourceResult holds data returned by a single DataSource.
// Each source populates only the fields it produces; nil/empty fields are ignored.
type SourceResult struct {
	Calendar     *db.CalendarData
	Todos        []db.Todo
	PullRequests []db.PullRequest
	Warnings     []db.Warning
}

// combineResults merges all SourceResults into a single FreshData.
// It strips ANSI escape sequences from all external string fields to prevent
// terminal injection attacks (e.g., malicious calendar titles setting terminal title).
func combineResults(results []*SourceResult) *FreshData {
	fresh := &FreshData{}
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Calendar != nil {
			cal := *r.Calendar
			sanitizeCalendarData(&cal)
			fresh.Calendar = cal
		}
		fresh.Todos = append(fresh.Todos, r.Todos...)
		fresh.PullRequests = append(fresh.PullRequests, r.PullRequests...)
	}
	sanitizeTodos(fresh.Todos)
	sanitizePullRequests(fresh.PullRequests)
	return fresh
}

// sanitizeCalendarData strips ANSI escapes from calendar event titles.
func sanitizeCalendarData(cal *db.CalendarData) {
	for i := range cal.Today {
		cal.Today[i].Title = sanitize.StripANSI(cal.Today[i].Title)
	}
	for i := range cal.Tomorrow {
		cal.Tomorrow[i].Title = sanitize.StripANSI(cal.Tomorrow[i].Title)
	}
}

// sanitizeTodos strips ANSI escapes from todo display fields.
func sanitizeTodos(todos []db.Todo) {
	for i := range todos {
		todos[i].Title = sanitize.StripANSI(todos[i].Title)
		todos[i].Context = sanitize.StripANSI(todos[i].Context)
		todos[i].Detail = sanitize.StripANSI(todos[i].Detail)
	}
}

// sanitizePullRequests strips ANSI escapes from PR display fields.
func sanitizePullRequests(prs []db.PullRequest) {
	for i := range prs {
		prs[i].Title = sanitize.StripANSI(prs[i].Title)
	}
}

