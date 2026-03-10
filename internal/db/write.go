package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Write methods -- Todos
// ---------------------------------------------------------------------------

func DBCompleteTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	if err != nil {
		return fmt.Errorf("complete todo %s: %w", id, err)
	}
	return nil
}

func DBDismissTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = 'dismissed', updated_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("dismiss todo %s: %w", id, err)
	}
	return nil
}

func DBRestoreTodo(db *sql.DB, id, status string, completedAt *time.Time) error {
	now := FormatTime(time.Now())
	var ca *string
	if completedAt != nil {
		s := FormatTime(*completedAt)
		ca = &s
	}
	_, err := db.Exec(`UPDATE cc_todos SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
		status, ca, now, id)
	if err != nil {
		return fmt.Errorf("restore todo %s: %w", id, err)
	}
	return nil
}

func DBDeferTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_todos WHERE status = 'active'), updated_at = ? WHERE id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("defer todo %s: %w", id, err)
	}
	return nil
}

func DBPromoteTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MIN(sort_order), 0) - 1 FROM cc_todos WHERE status = 'active'), updated_at = ? WHERE id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("promote todo %s: %w", id, err)
	}
	return nil
}

func DBInsertTodo(db *sql.DB, t Todo) error {
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
	_, err := db.Exec(`INSERT INTO cc_todos (id, title, status, source, source_ref, context, detail,
		who_waiting, project_dir, due, effort, sort_order, created_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, ''),
		(SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_todos WHERE status = 'active'),
		?, ?, ?)`,
		t.ID, t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
		t.WhoWaiting, t.ProjectDir, t.Due, t.Effort,
		createdAt, completedAt, now)
	if err != nil {
		return fmt.Errorf("insert todo %s: %w", t.ID, err)
	}
	return nil
}

func DBUpdateTodo(db *sql.DB, id string, t Todo) error {
	now := FormatTime(time.Now())
	var completedAt *string
	if t.CompletedAt != nil {
		s := FormatTime(*t.CompletedAt)
		completedAt = &s
	}
	_, err := db.Exec(`UPDATE cc_todos SET title = ?, status = ?, source = ?,
		source_ref = NULLIF(?, ''), context = NULLIF(?, ''), detail = NULLIF(?, ''),
		who_waiting = NULLIF(?, ''), project_dir = NULLIF(?, ''), due = NULLIF(?, ''),
		effort = NULLIF(?, ''), completed_at = ?, updated_at = ?
		WHERE id = ?`,
		t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
		t.WhoWaiting, t.ProjectDir, t.Due, t.Effort, completedAt, now, id)
	if err != nil {
		return fmt.Errorf("update todo %s: %w", id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Calendar & Suggestions
// ---------------------------------------------------------------------------

func DBReplaceCalendar(db *sql.DB, cal CalendarData) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM cc_calendar_cache`); err != nil {
		return err
	}

	now := FormatTime(time.Now())
	stmt, err := tx.Prepare(`INSERT INTO cc_calendar_cache (day, title, start_time, end_time, all_day, declined, calendar_id, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range cal.Today {
		if _, err := stmt.Exec("today", ev.Title, FormatTime(ev.Start), FormatTime(ev.End), ev.AllDay, ev.Declined, ev.CalendarID, now); err != nil {
			return err
		}
	}
	for _, ev := range cal.Tomorrow {
		if _, err := stmt.Exec("tomorrow", ev.Title, FormatTime(ev.Start), FormatTime(ev.End), ev.AllDay, ev.Declined, ev.CalendarID, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func DBSaveFocus(db *sql.DB, focus string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_suggestions (id, focus, ranked_todo_ids, reasons, updated_at)
		VALUES (1, ?,
			COALESCE((SELECT ranked_todo_ids FROM cc_suggestions WHERE id = 1), '[]'),
			COALESCE((SELECT reasons FROM cc_suggestions WHERE id = 1), '{}'),
			?)`, focus, now)
	if err != nil {
		return fmt.Errorf("save focus: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Threads
// ---------------------------------------------------------------------------

func DBPauseThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'paused', paused_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	if err != nil {
		return fmt.Errorf("pause thread %s: %w", id, err)
	}
	return nil
}

func DBStartThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'active', paused_at = NULL, updated_at = ? WHERE id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("start thread %s: %w", id, err)
	}
	return nil
}

func DBCloseThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	if err != nil {
		return fmt.Errorf("close thread %s: %w", id, err)
	}
	return nil
}

func DBInsertThread(db *sql.DB, t Thread) error {
	now := FormatTime(time.Now())
	createdAt := FormatTime(t.CreatedAt)
	if t.CreatedAt.IsZero() {
		createdAt = now
	}
	_, err := db.Exec(`INSERT INTO cc_threads (id, type, title, url, repo, project_dir, status, summary,
		source_ref, created_at, paused_at, completed_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''),
		NULLIF(?, ''), ?, NULL, NULL, ?)`,
		t.ID, t.Type, t.Title, t.URL, t.Repo, t.ProjectDir, t.Status, t.Summary,
		"", createdAt, now)
	if err != nil {
		return fmt.Errorf("insert thread %s: %w", t.ID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Pending Actions
// ---------------------------------------------------------------------------

func DBInsertPendingAction(db *sql.DB, a PendingAction) error {
	_, err := db.Exec(`INSERT INTO cc_pending_actions (type, todo_id, duration_minutes, requested_at)
		VALUES (?, ?, ?, ?)`,
		a.Type, a.TodoID, a.DurationMinutes, FormatTime(a.RequestedAt))
	return err
}

// ---------------------------------------------------------------------------
// Write methods -- Bookmarks
// ---------------------------------------------------------------------------

func DBInsertBookmark(db *sql.DB, b Session) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_bookmarks (session_id, project, repo, branch, label, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.SessionID, b.Project, b.Repo, b.Branch, "", b.Summary, FormatTime(b.Created))
	if err != nil {
		return fmt.Errorf("insert bookmark %s: %w", b.SessionID, err)
	}
	return nil
}

func DBRemoveBookmark(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM cc_bookmarks WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("remove bookmark %s: %w", sessionID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Learned Paths
// ---------------------------------------------------------------------------

func DBAddPath(db *sql.DB, path string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO cc_learned_paths (path, added_at) VALUES (?, ?)`,
		path, FormatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("add path %s: %w", path, err)
	}
	return nil
}

func DBRemovePath(db *sql.DB, path string) error {
	_, err := db.Exec(`DELETE FROM cc_learned_paths WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("remove path %s: %w", path, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Bulk refresh result
// ---------------------------------------------------------------------------

// DBSaveRefreshResult atomically replaces all refresh-managed data (todos,
// threads, calendar, suggestions, pending actions, generated_at) in a single
// transaction. This is the write path used by ccc-refresh.
func DBSaveRefreshResult(d *sql.DB, cc *CommandCenter) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := FormatTime(time.Now())

	// --- Todos: delete all, re-insert ---
	if _, err := tx.Exec(`DELETE FROM cc_todos`); err != nil {
		return fmt.Errorf("clear todos: %w", err)
	}
	for i, t := range cc.Todos {
		createdAt := FormatTime(t.CreatedAt)
		if t.CreatedAt.IsZero() {
			createdAt = now
		}
		var completedAt *string
		if t.CompletedAt != nil {
			s := FormatTime(*t.CompletedAt)
			completedAt = &s
		}
		_, err := tx.Exec(`INSERT INTO cc_todos (id, title, status, source, source_ref, context, detail,
			who_waiting, project_dir, due, effort, sort_order, created_at, completed_at, updated_at)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`,
			t.ID, t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
			t.WhoWaiting, t.ProjectDir, t.Due, t.Effort, i,
			createdAt, completedAt, now)
		if err != nil {
			return fmt.Errorf("insert todo %s: %w", t.ID, err)
		}
	}

	// --- Threads: delete all, re-insert ---
	if _, err := tx.Exec(`DELETE FROM cc_threads`); err != nil {
		return fmt.Errorf("clear threads: %w", err)
	}
	for _, t := range cc.Threads {
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
		_, err := tx.Exec(`INSERT INTO cc_threads (id, type, title, url, repo, project_dir, status, summary,
			source_ref, created_at, paused_at, completed_at, updated_at)
			VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''),
			NULLIF(?, ''), ?, ?, ?, ?)`,
			t.ID, t.Type, t.Title, t.URL, t.Repo, t.ProjectDir, t.Status, t.Summary,
			"", createdAt, pausedAt, completedAt, now)
		if err != nil {
			return fmt.Errorf("insert thread %s: %w", t.ID, err)
		}
	}

	// --- Calendar: replace ---
	if _, err := tx.Exec(`DELETE FROM cc_calendar_cache`); err != nil {
		return fmt.Errorf("clear calendar: %w", err)
	}
	for _, ev := range cc.Calendar.Today {
		if _, err := tx.Exec(`INSERT INTO cc_calendar_cache (day, title, start_time, end_time, all_day, declined, calendar_id, cached_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"today", ev.Title, FormatTime(ev.Start), FormatTime(ev.End), ev.AllDay, ev.Declined, ev.CalendarID, now); err != nil {
			return fmt.Errorf("insert calendar event: %w", err)
		}
	}
	for _, ev := range cc.Calendar.Tomorrow {
		if _, err := tx.Exec(`INSERT INTO cc_calendar_cache (day, title, start_time, end_time, all_day, declined, calendar_id, cached_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"tomorrow", ev.Title, FormatTime(ev.Start), FormatTime(ev.End), ev.AllDay, ev.Declined, ev.CalendarID, now); err != nil {
			return fmt.Errorf("insert calendar event: %w", err)
		}
	}

	// --- Suggestions ---
	rankedJSON, _ := json.Marshal(cc.Suggestions.RankedTodoIDs)
	reasonsJSON, _ := json.Marshal(cc.Suggestions.Reasons)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO cc_suggestions (id, focus, ranked_todo_ids, reasons, updated_at)
		VALUES (1, ?, ?, ?, ?)`,
		cc.Suggestions.Focus, string(rankedJSON), string(reasonsJSON), now); err != nil {
		return fmt.Errorf("save suggestions: %w", err)
	}

	// --- Pending actions ---
	if _, err := tx.Exec(`DELETE FROM cc_pending_actions`); err != nil {
		return fmt.Errorf("clear pending actions: %w", err)
	}
	for _, a := range cc.PendingActions {
		if _, err := tx.Exec(`INSERT INTO cc_pending_actions (type, todo_id, duration_minutes, requested_at)
			VALUES (?, ?, ?, ?)`,
			a.Type, a.TodoID, a.DurationMinutes, FormatTime(a.RequestedAt)); err != nil {
			return fmt.Errorf("insert pending action: %w", err)
		}
	}

	// --- Generated at ---
	if _, err := tx.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at)
		VALUES ('generated_at', ?, ?)`,
		FormatTime(cc.GeneratedAt), now); err != nil {
		return fmt.Errorf("save generated_at: %w", err)
	}

	return tx.Commit()
}

// DBSaveSuggestions replaces the suggestions row.
func DBSaveSuggestions(d *sql.DB, s Suggestions) error {
	now := FormatTime(time.Now())
	rankedJSON, _ := json.Marshal(s.RankedTodoIDs)
	reasonsJSON, _ := json.Marshal(s.Reasons)
	_, err := d.Exec(`INSERT OR REPLACE INTO cc_suggestions (id, focus, ranked_todo_ids, reasons, updated_at)
		VALUES (1, ?, ?, ?, ?)`,
		s.Focus, string(rankedJSON), string(reasonsJSON), now)
	return err
}

// DBSetMeta upserts a key-value pair in cc_meta.
func DBSetMeta(d *sql.DB, key, value string) error {
	now := FormatTime(time.Now())
	_, err := d.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at) VALUES (?, ?, ?)`,
		key, value, now)
	return err
}

// DBClearPendingActions removes all pending actions.
func DBClearPendingActions(d *sql.DB) error {
	_, err := d.Exec(`DELETE FROM cc_pending_actions`)
	return err
}
