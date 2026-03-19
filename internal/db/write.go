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
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_todos WHERE status NOT IN ('completed', 'dismissed')), updated_at = ? WHERE id = ?`,
		now, id)
	if err != nil {
		return fmt.Errorf("defer todo %s: %w", id, err)
	}
	return nil
}

func DBPromoteTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MIN(sort_order), 0) - 1 FROM cc_todos WHERE status NOT IN ('completed', 'dismissed')), updated_at = ? WHERE id = ?`,
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
		who_waiting, project_dir, due, effort, session_id, proposed_prompt, session_summary,
		session_log_path, source_context, source_context_at,
		display_id, sort_order, created_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		(SELECT COALESCE(MAX(display_id), 0) + 1 FROM cc_todos),
		(SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_todos WHERE status NOT IN ('completed', 'dismissed')),
		?, ?, ?)`,
		t.ID, t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
		t.WhoWaiting, t.ProjectDir, t.Due, t.Effort, t.SessionID, t.ProposedPrompt, t.SessionSummary,
		t.SessionLogPath, t.SourceContext, t.SourceContextAt,
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
		effort = NULLIF(?, ''), session_id = NULLIF(?, ''),
		proposed_prompt = NULLIF(?, ''),
		session_summary = NULLIF(?, ''), session_log_path = NULLIF(?, ''),
		source_context = NULLIF(?, ''),
		source_context_at = NULLIF(?, ''),
		completed_at = ?, updated_at = ?
		WHERE id = ?`,
		t.Title, t.Status, t.Source, t.SourceRef, t.Context, t.Detail,
		t.WhoWaiting, t.ProjectDir, t.Due, t.Effort, t.SessionID,
		t.ProposedPrompt, t.SessionSummary, t.SessionLogPath,
		t.SourceContext, t.SourceContextAt, completedAt, now, id)
	if err != nil {
		return fmt.Errorf("update todo %s: %w", id, err)
	}
	return nil
}

// DBAcceptTodo transitions a todo from "new" to "backlog".
func DBAcceptTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = ?, updated_at = ? WHERE id = ?`,
		StatusBacklog, now, id)
	if err != nil {
		return fmt.Errorf("accept todo %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoStatus updates the status column for a todo.
func DBUpdateTodoStatus(db *sql.DB, id string, status string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = ?, updated_at = ? WHERE id = ?`,
		status, now, id)
	if err != nil {
		return fmt.Errorf("update todo status %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoProjectDir updates only the project_dir column for a todo.
func DBUpdateTodoProjectDir(db *sql.DB, id string, projectDir string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET project_dir = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		projectDir, now, id)
	if err != nil {
		return fmt.Errorf("update todo project_dir %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoLaunchMode updates only the launch_mode column for a todo.
func DBUpdateTodoLaunchMode(db *sql.DB, id string, launchMode string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET launch_mode = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		launchMode, now, id)
	if err != nil {
		return fmt.Errorf("update todo launch_mode %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoSessionID updates only the session_id column for a todo.
func DBUpdateTodoSessionID(db *sql.DB, id string, sessionID string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET session_id = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		sessionID, now, id)
	if err != nil {
		return fmt.Errorf("update todo session_id %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoSessionSummary updates only the session_summary column for a todo.
func DBUpdateTodoSessionSummary(db *sql.DB, id string, summary string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET session_summary = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		summary, now, id)
	if err != nil {
		return fmt.Errorf("update todo session summary %s: %w", id, err)
	}
	return nil
}

// DBUpdateTodoSourceContext updates only the source_context columns for a todo.
func DBUpdateTodoSourceContext(db *sql.DB, id, sourceContext, sourceContextAt string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET source_context = NULLIF(?, ''), source_context_at = NULLIF(?, ''), updated_at = ? WHERE id = ?`,
		sourceContext, sourceContextAt, now, id)
	if err != nil {
		return fmt.Errorf("update todo source_context %s: %w", id, err)
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
// Write methods -- Pending Actions
// ---------------------------------------------------------------------------

func DBInsertPendingAction(db *sql.DB, a PendingAction) error {
	_, err := db.Exec(`INSERT INTO cc_pending_actions (type, todo_id, duration_minutes, requested_at)
		VALUES (?, ?, ?, ?)`,
		a.Type, a.TodoID, a.DurationMinutes, FormatTime(a.RequestedAt))
	if err != nil {
		return fmt.Errorf("insert pending action %s: %w", a.TodoID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Write methods -- Bookmarks
// ---------------------------------------------------------------------------

func DBInsertBookmark(db *sql.DB, b Session, label string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_bookmarks (session_id, project, repo, branch, label, summary, created_at, worktree_path, source_repo)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))`,
		b.SessionID, b.Project, b.Repo, b.Branch, label, b.Summary, FormatTime(b.Created),
		b.WorktreePath, b.SourceRepo)
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
	_, err := db.Exec(`INSERT OR IGNORE INTO cc_learned_paths (path, added_at, sort_order) VALUES (?, ?,
		(SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_learned_paths))`,
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

// DBUpdatePathDescription updates the description for a learned path.
func DBUpdatePathDescription(db *sql.DB, path, description string) error {
	res, err := db.Exec(`UPDATE cc_learned_paths SET description = ? WHERE path = ?`, description, path)
	if err != nil {
		return fmt.Errorf("update path description %s: %w", path, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("path not found: %s", path)
	}
	return nil
}

// DBSwapPathOrder swaps the sort_order of two paths.
func DBSwapPathOrder(database *sql.DB, pathA, pathB string) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("swap path order: begin tx: %w", err)
	}
	defer tx.Rollback()

	var orderA, orderB int
	if err := tx.QueryRow(`SELECT sort_order FROM cc_learned_paths WHERE path = ?`, pathA).Scan(&orderA); err != nil {
		return fmt.Errorf("swap path order: read A: %w", err)
	}
	if err := tx.QueryRow(`SELECT sort_order FROM cc_learned_paths WHERE path = ?`, pathB).Scan(&orderB); err != nil {
		return fmt.Errorf("swap path order: read B: %w", err)
	}

	if _, err := tx.Exec(`UPDATE cc_learned_paths SET sort_order = ? WHERE path = ?`, orderB, pathA); err != nil {
		return fmt.Errorf("swap path order: write A: %w", err)
	}
	if _, err := tx.Exec(`UPDATE cc_learned_paths SET sort_order = ? WHERE path = ?`, orderA, pathB); err != nil {
		return fmt.Errorf("swap path order: write B: %w", err)
	}

	return tx.Commit()
}

// DBSwapTodoOrder swaps the sort_order of two todos by ID.
func DBSwapTodoOrder(database *sql.DB, idA, idB string) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("swap todo order: begin tx: %w", err)
	}
	defer tx.Rollback()

	var orderA, orderB int
	if err := tx.QueryRow(`SELECT sort_order FROM cc_todos WHERE id = ?`, idA).Scan(&orderA); err != nil {
		return fmt.Errorf("swap todo order: read A: %w", err)
	}
	if err := tx.QueryRow(`SELECT sort_order FROM cc_todos WHERE id = ?`, idB).Scan(&orderB); err != nil {
		return fmt.Errorf("swap todo order: read B: %w", err)
	}

	now := FormatTime(time.Now())
	if _, err := tx.Exec(`UPDATE cc_todos SET sort_order = ?, updated_at = ? WHERE id = ?`, orderB, now, idA); err != nil {
		return fmt.Errorf("swap todo order: write A: %w", err)
	}
	if _, err := tx.Exec(`UPDATE cc_todos SET sort_order = ?, updated_at = ? WHERE id = ?`, orderA, now, idB); err != nil {
		return fmt.Errorf("swap todo order: write B: %w", err)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Write methods -- Bulk refresh result
// ---------------------------------------------------------------------------

// DBSaveRefreshResult atomically replaces all refresh-managed data (todos,
// calendar, suggestions, pending actions, generated_at) in a single
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
	// Find the current max display_id so we can assign IDs to new todos.
	maxDisplayID := 0
	for _, t := range cc.Todos {
		if t.DisplayID > maxDisplayID {
			maxDisplayID = t.DisplayID
		}
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
		// Assign a display_id to new todos that don't have one yet.
		displayID := t.DisplayID
		if displayID == 0 {
			maxDisplayID++
			displayID = maxDisplayID
		}
		_, err := tx.Exec(`INSERT INTO cc_todos (id, title, status, source, source_ref, context, detail,
			who_waiting, project_dir, launch_mode, due, effort, session_id, proposed_prompt, session_summary,
			session_log_path, source_context, source_context_at,
			display_id, sort_order, created_at, completed_at, updated_at)
			VALUES (?, ?, ?, ?,
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''),
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			?, ?, ?, ?, ?)`,
			t.ID, t.Title, t.Status, t.Source,
			t.SourceRef, t.Context, t.Detail,
			t.WhoWaiting, t.ProjectDir, t.LaunchMode,
			t.Due, t.Effort, t.SessionID,
			t.ProposedPrompt, t.SessionSummary,
			t.SessionLogPath, t.SourceContext, t.SourceContextAt,
			displayID, i, createdAt, completedAt, now)
		if err != nil {
			return fmt.Errorf("insert todo %s: %w", t.ID, err)
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
	if err != nil {
		return fmt.Errorf("save suggestions: %w", err)
	}
	return nil
}

// DBSetMeta upserts a key-value pair in cc_meta.
func DBSetMeta(d *sql.DB, key, value string) error {
	now := FormatTime(time.Now())
	_, err := d.Exec(`INSERT OR REPLACE INTO cc_meta (key, value, updated_at) VALUES (?, ?, ?)`,
		key, value, now)
	if err != nil {
		return fmt.Errorf("set meta %s: %w", key, err)
	}
	return nil
}

// DBUpsertSourceSync records a sync result (success or failure) for a data source.
func DBUpsertSourceSync(d *sql.DB, source string, syncErr error) error {
	now := FormatTime(time.Now())
	if syncErr == nil {
		// Success: update last_success, clear last_error
		_, err := d.Exec(`INSERT OR REPLACE INTO cc_source_sync (source, last_success, last_error, updated_at)
			VALUES (?, ?, '', ?)`, source, now, now)
		if err != nil {
			return fmt.Errorf("upsert source sync %s: %w", source, err)
		}
	} else {
		// Failure: keep existing last_success, update last_error
		_, err := d.Exec(`INSERT INTO cc_source_sync (source, last_success, last_error, updated_at)
			VALUES (?, NULL, ?, ?)
			ON CONFLICT(source) DO UPDATE SET last_error = ?, updated_at = ?`,
			source, syncErr.Error(), now, syncErr.Error(), now)
		if err != nil {
			return fmt.Errorf("upsert source sync %s: %w", source, err)
		}
	}
	return nil
}

// DBClearPendingActions removes all pending actions.
func DBClearPendingActions(d *sql.DB) error {
	_, err := d.Exec(`DELETE FROM cc_pending_actions`)
	if err != nil {
		return fmt.Errorf("clear pending actions: %w", err)
	}
	return nil
}
