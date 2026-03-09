package refresh

import (
	"context"

	"github.com/anutron/claude-command-center/internal/db"
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

// SourceResult holds data returned by a single DataSource.
// Each source populates only the fields it produces; nil/empty fields are ignored.
type SourceResult struct {
	Calendar *db.CalendarData
	Todos    []db.Todo
	Threads  []db.Thread
	Warnings []db.Warning
}

// combineResults merges all SourceResults into a single FreshData.
func combineResults(results []*SourceResult) *FreshData {
	fresh := &FreshData{}
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Calendar != nil {
			fresh.Calendar = *r.Calendar
		}
		fresh.Todos = append(fresh.Todos, r.Todos...)
		fresh.Threads = append(fresh.Threads, r.Threads...)
	}
	return fresh
}
