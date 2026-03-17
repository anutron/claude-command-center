package refresh

import (
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

// Merge combines fresh data with existing command center state.
// Rules:
//   - Calendar: replaced entirely (always fresh)
//   - Todos: merge by source_ref; dismissed = tombstone (never recreate);
//     update existing (preserve ID/status/created_at, update detail);
//     add new with generated UUID; preserve source:"manual" untouched
func Merge(existing *db.CommandCenter, fresh *FreshData) *db.CommandCenter {
	if existing == nil {
		existing = &db.CommandCenter{}
	}

	cc := &db.CommandCenter{
		GeneratedAt:    time.Now(),
		Calendar:       fresh.Calendar,
		PendingActions: existing.PendingActions,
		Suggestions:    existing.Suggestions,
	}

	cc.Todos = mergeTodos(existing.Todos, fresh.Todos)

	return cc
}

func mergeTodos(existing, fresh []db.Todo) []db.Todo {
	byRef := make(map[string]int)
	for i, t := range existing {
		if t.SourceRef != "" {
			byRef[t.SourceRef] = i
		}
	}

	matched := make(map[int]bool)

	var merged []db.Todo
	for _, ft := range fresh {
		if ft.SourceRef == "" {
			if ft.ID == "" {
				ft.ID = db.GenID()
			}
			if ft.CreatedAt.IsZero() {
				ft.CreatedAt = time.Now()
			}
			merged = append(merged, ft)
			continue
		}

		if idx, ok := byRef[ft.SourceRef]; ok {
			et := existing[idx]
			matched[idx] = true

			if et.Status == "dismissed" {
				continue // tombstone — never recreate
			}
			if et.Status == "completed" {
				merged = append(merged, et) // preserve as-is, don't overwrite
				continue
			}

			et.Title = ft.Title
			et.Detail = ft.Detail
			et.Context = ft.Context
			et.WhoWaiting = ft.WhoWaiting
			if ft.Due != "" {
				et.Due = ft.Due
			}
			if ft.Effort != "" {
				et.Effort = ft.Effort
			}
			// Preserve proposed_prompt if already set; only write if currently empty
			if et.ProposedPrompt == "" && ft.ProposedPrompt != "" {
				et.ProposedPrompt = ft.ProposedPrompt
			}
			// Always preserve session_status (refresh never overwrites it)
			// Always preserve triage_status (refresh never overwrites it)
			merged = append(merged, et)
		} else {
			if ft.ID == "" {
				ft.ID = db.GenID()
			}
			if ft.CreatedAt.IsZero() {
				ft.CreatedAt = time.Now()
			}
			if ft.Status == "" {
				ft.Status = "active"
			}
			// New external todos start in triage
			if ft.TriageStatus == "" {
				ft.TriageStatus = "new"
			}
			merged = append(merged, ft)
		}
	}

	for i, t := range existing {
		if !matched[i] {
			merged = append(merged, t)
		}
	}

	return merged
}

