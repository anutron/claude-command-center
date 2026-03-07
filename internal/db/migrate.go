package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// MigrateFromJSON imports existing flat file state into the SQLite database.
// It is idempotent -- uses INSERT OR IGNORE so existing rows are not overwritten.
// Only runs if the database is empty (no todos yet) and JSON files exist.
func MigrateFromJSON(db *sql.DB, ccPath, bookmarksPath, pathsPath string) error {
	if !DBIsEmpty(db) {
		return nil // already has data
	}

	var migrated []string

	if err := migrateCommandCenter(db, ccPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not migrate command-center.json: %v\n", err)
	} else {
		migrated = append(migrated, "command-center")
	}

	if err := migrateBookmarks(db, bookmarksPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not migrate bookmarks.json: %v\n", err)
	} else {
		migrated = append(migrated, "bookmarks")
	}

	if err := migrateLearnedPaths(db, pathsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not migrate learned-paths.txt: %v\n", err)
	} else {
		migrated = append(migrated, "learned-paths")
	}

	if len(migrated) > 0 {
		fmt.Fprintf(os.Stderr, "Migrated to SQLite: %v\n", migrated)
	}
	return nil
}

func migrateCommandCenter(db *sql.DB, ccPath string) error {
	data, err := os.ReadFile(ccPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var cc CommandCenter
	if err := json.Unmarshal(data, &cc); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Migrate todos with sort_order preserving array position
	for i, t := range cc.Todos {
		now := FormatTime(time.Now())
		createdAt := FormatTime(t.CreatedAt)
		if t.CreatedAt.IsZero() {
			createdAt = now
		}
		var completedAt *string
		if t.CompletedAt != nil {
			s := FormatTime(*t.CompletedAt)
			completedAt = &s
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO cc_todos
			(id, title, status, source, source_ref, context, detail, who_waiting,
			project_dir, due, effort, sort_order, created_at, completed_at, updated_at)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`,
			t.ID, t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
			t.WhoWaiting, t.ProjectDir, t.Due, t.Effort, i,
			createdAt, completedAt, now)
		if err != nil {
			return fmt.Errorf("todo %s: %w", t.ID, err)
		}
	}

	// Migrate threads
	for _, t := range cc.Threads {
		now := FormatTime(time.Now())
		createdAt := FormatTime(t.CreatedAt)
		if t.CreatedAt.IsZero() {
			createdAt = now
		}
		var pausedAt, completedAt *string
		if t.PausedAt != nil {
			s := FormatTime(*t.PausedAt)
			pausedAt = &s
		}
		if t.CompletedAt != nil {
			s := FormatTime(*t.CompletedAt)
			completedAt = &s
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO cc_threads
			(id, type, title, url, repo, project_dir, status, summary,
			created_at, paused_at, completed_at, updated_at)
			VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''),
			?, ?, ?, ?)`,
			t.ID, t.Type, t.Title, t.URL, t.Repo, t.ProjectDir, t.Status, t.Summary,
			createdAt, pausedAt, completedAt, now)
		if err != nil {
			return fmt.Errorf("thread %s: %w", t.ID, err)
		}
	}

	// Migrate calendar cache
	for _, ev := range cc.Calendar.Today {
		migrateCachedEvent(tx, "today", ev)
	}
	for _, ev := range cc.Calendar.Tomorrow {
		migrateCachedEvent(tx, "tomorrow", ev)
	}

	// Migrate suggestions
	if cc.Suggestions.Focus != "" || len(cc.Suggestions.RankedTodoIDs) > 0 {
		rankedJSON, _ := json.Marshal(cc.Suggestions.RankedTodoIDs)
		reasonsJSON, _ := json.Marshal(cc.Suggestions.Reasons)
		_, _ = tx.Exec(`INSERT OR REPLACE INTO cc_suggestions (id, focus, ranked_todo_ids, reasons, updated_at)
			VALUES (1, ?, ?, ?, ?)`,
			cc.Suggestions.Focus, string(rankedJSON), string(reasonsJSON), FormatTime(time.Now()))
	}

	// Migrate pending actions
	for _, a := range cc.PendingActions {
		_, _ = tx.Exec(`INSERT INTO cc_pending_actions (type, todo_id, duration_minutes, requested_at)
			VALUES (?, ?, ?, ?)`,
			a.Type, a.TodoID, a.DurationMinutes, FormatTime(a.RequestedAt))
	}

	// Migrate generated_at
	if !cc.GeneratedAt.IsZero() {
		_, _ = tx.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at)
			VALUES ('generated_at', ?, ?)`,
			FormatTime(cc.GeneratedAt), FormatTime(time.Now()))
	}

	return tx.Commit()
}

func migrateCachedEvent(tx *sql.Tx, day string, ev CalendarEvent) {
	_, _ = tx.Exec(`INSERT INTO cc_calendar_cache (day, title, start_time, end_time, all_day, declined, calendar_id, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		day, ev.Title, FormatTime(ev.Start), FormatTime(ev.End), ev.AllDay, ev.Declined, ev.CalendarID, FormatTime(time.Now()))
}

func migrateBookmarks(db *sql.DB, bookmarksPath string) error {
	data, err := os.ReadFile(bookmarksPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var bookmarks []Bookmark
	if err := json.Unmarshal(data, &bookmarks); err != nil {
		return err
	}

	for _, b := range bookmarks {
		_, err := db.Exec(`INSERT OR IGNORE INTO cc_bookmarks (session_id, project, repo, branch, label, summary, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			b.SessionID, b.Project, b.Repo, b.Branch, b.Label, b.Summary, b.Created)
		if err != nil {
			return err
		}
	}
	return nil
}

func migrateLearnedPaths(db *sql.DB, pathsPath string) error {
	paths, err := LoadPaths(pathsPath)
	if err != nil || len(paths) == 0 {
		return err
	}

	for _, p := range paths {
		_, err := db.Exec(`INSERT OR IGNORE INTO cc_learned_paths (path, added_at) VALUES (?, ?)`,
			p, FormatTime(time.Now()))
		if err != nil {
			return err
		}
	}
	return nil
}
