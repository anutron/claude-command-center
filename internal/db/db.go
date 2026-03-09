package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// OpenDB opens (or creates) the SQLite database at dbPath, enables WAL mode,
// and runs the idempotent schema migration.
func OpenDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrateSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cc_todos (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			source TEXT NOT NULL DEFAULT 'manual',
			source_ref TEXT,
			context TEXT,
			detail TEXT,
			who_waiting TEXT,
			project_dir TEXT,
			due TEXT,
			effort TEXT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			updated_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_cc_todos_source_ref
			ON cc_todos(source_ref) WHERE source_ref IS NOT NULL AND source_ref != '';

		CREATE TABLE IF NOT EXISTS cc_threads (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL DEFAULT 'manual',
			title TEXT NOT NULL,
			url TEXT,
			repo TEXT,
			project_dir TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			summary TEXT,
			source_ref TEXT,
			created_at TEXT NOT NULL,
			paused_at TEXT,
			completed_at TEXT,
			updated_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_cc_threads_source_ref
			ON cc_threads(source_ref) WHERE source_ref IS NOT NULL AND source_ref != '';

		CREATE TABLE IF NOT EXISTS cc_calendar_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			day TEXT NOT NULL,
			title TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			all_day INTEGER NOT NULL DEFAULT 0,
			declined INTEGER NOT NULL DEFAULT 0,
			calendar_id TEXT NOT NULL DEFAULT '',
			cached_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_suggestions (
			id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			focus TEXT,
			ranked_todo_ids TEXT DEFAULT '[]',
			reasons TEXT DEFAULT '{}',
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_pending_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			todo_id TEXT NOT NULL,
			duration_minutes INTEGER,
			requested_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_meta (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_bookmarks (
			session_id TEXT PRIMARY KEY,
			project TEXT,
			repo TEXT,
			branch TEXT,
			label TEXT,
			summary TEXT,
			created_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS cc_learned_paths (
			path TEXT PRIMARY KEY,
			added_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add calendar_id column if missing (added after initial schema)
	_, _ = db.Exec(`ALTER TABLE cc_calendar_cache ADD COLUMN calendar_id TEXT NOT NULL DEFAULT ''`)

	return nil
}

// ---------------------------------------------------------------------------
// Read methods
// ---------------------------------------------------------------------------

func LoadCommandCenterFromDB(db *sql.DB) (*CommandCenter, error) {
	cc := &CommandCenter{}

	todos, err := dbLoadTodos(db)
	if err != nil {
		return nil, fmt.Errorf("load todos: %w", err)
	}
	cc.Todos = todos

	threads, err := dbLoadThreads(db)
	if err != nil {
		return nil, fmt.Errorf("load threads: %w", err)
	}
	cc.Threads = threads

	cal, err := dbLoadCalendar(db)
	if err != nil {
		return nil, fmt.Errorf("load calendar: %w", err)
	}
	cc.Calendar = cal

	sug, err := dbLoadSuggestions(db)
	if err != nil {
		return nil, fmt.Errorf("load suggestions: %w", err)
	}
	cc.Suggestions = sug

	actions, err := dbLoadPendingActions(db)
	if err != nil {
		return nil, fmt.Errorf("load pending actions: %w", err)
	}
	cc.PendingActions = actions

	genAt, err := dbLoadGeneratedAt(db)
	if err == nil {
		cc.GeneratedAt = genAt
	}

	return cc, nil
}

func dbLoadTodos(db *sql.DB) ([]Todo, error) {
	rows, err := db.Query(`SELECT id, title, status, source, source_ref, context, detail,
		who_waiting, project_dir, due, effort, created_at, completed_at
		FROM cc_todos ORDER BY sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		var createdStr string
		var completedStr sql.NullString
		var sourceRef, ctx, detail, who, projDir, due, effort sql.NullString

		err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Source,
			&sourceRef, &ctx, &detail, &who, &projDir, &due, &effort,
			&createdStr, &completedStr)
		if err != nil {
			return nil, err
		}

		t.SourceRef = sourceRef.String
		t.Context = ctx.String
		t.Detail = detail.String
		t.WhoWaiting = who.String
		t.ProjectDir = projDir.String
		t.Due = due.String
		t.Effort = effort.String
		t.CreatedAt = ParseTime(createdStr)
		if completedStr.Valid {
			ct := ParseTime(completedStr.String)
			t.CompletedAt = &ct
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func dbLoadThreads(db *sql.DB) ([]Thread, error) {
	rows, err := db.Query(`SELECT id, type, title, url, repo, project_dir, status, summary,
		created_at, paused_at, completed_at
		FROM cc_threads ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []Thread
	for rows.Next() {
		var t Thread
		var createdStr string
		var url, repo, projDir, summary sql.NullString
		var pausedStr, completedStr sql.NullString

		err := rows.Scan(&t.ID, &t.Type, &t.Title, &url, &repo, &projDir,
			&t.Status, &summary, &createdStr, &pausedStr, &completedStr)
		if err != nil {
			return nil, err
		}

		t.URL = url.String
		t.Repo = repo.String
		t.ProjectDir = projDir.String
		t.Summary = summary.String
		t.CreatedAt = ParseTime(createdStr)
		if pausedStr.Valid {
			pt := ParseTime(pausedStr.String)
			t.PausedAt = &pt
		}
		if completedStr.Valid {
			ct := ParseTime(completedStr.String)
			t.CompletedAt = &ct
		}
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

func dbLoadCalendar(db *sql.DB) (CalendarData, error) {
	cal := CalendarData{}
	rows, err := db.Query(`SELECT day, title, start_time, end_time, all_day, declined, calendar_id
		FROM cc_calendar_cache ORDER BY start_time ASC`)
	if err != nil {
		return cal, err
	}
	defer rows.Close()

	for rows.Next() {
		var day, title, startStr, endStr, calendarID string
		var allDay, declined bool
		if err := rows.Scan(&day, &title, &startStr, &endStr, &allDay, &declined, &calendarID); err != nil {
			return cal, err
		}
		ev := CalendarEvent{
			Title:      title,
			Start:      ParseTime(startStr),
			End:        ParseTime(endStr),
			AllDay:     allDay,
			Declined:   declined,
			CalendarID: calendarID,
		}
		if day == "today" {
			cal.Today = append(cal.Today, ev)
		} else {
			cal.Tomorrow = append(cal.Tomorrow, ev)
		}
	}
	return cal, rows.Err()
}

func dbLoadSuggestions(db *sql.DB) (Suggestions, error) {
	var s Suggestions
	var rankedJSON, reasonsJSON sql.NullString
	var focus sql.NullString
	err := db.QueryRow(`SELECT focus, ranked_todo_ids, reasons FROM cc_suggestions WHERE id = 1`).
		Scan(&focus, &rankedJSON, &reasonsJSON)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.Focus = focus.String
	if rankedJSON.Valid {
		_ = json.Unmarshal([]byte(rankedJSON.String), &s.RankedTodoIDs)
	}
	if reasonsJSON.Valid {
		_ = json.Unmarshal([]byte(reasonsJSON.String), &s.Reasons)
	}
	return s, nil
}

func dbLoadPendingActions(db *sql.DB) ([]PendingAction, error) {
	rows, err := db.Query(`SELECT type, todo_id, duration_minutes, requested_at FROM cc_pending_actions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []PendingAction
	for rows.Next() {
		var a PendingAction
		var reqStr string
		var dur sql.NullInt64
		if err := rows.Scan(&a.Type, &a.TodoID, &dur, &reqStr); err != nil {
			return nil, err
		}
		if dur.Valid {
			a.DurationMinutes = int(dur.Int64)
		}
		a.RequestedAt = ParseTime(reqStr)
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

func dbLoadGeneratedAt(db *sql.DB) (time.Time, error) {
	var val sql.NullString
	err := db.QueryRow(`SELECT value FROM cc_meta WHERE key = 'generated_at'`).Scan(&val)
	if err != nil || !val.Valid {
		return time.Time{}, err
	}
	return ParseTime(val.String), nil
}

// ---------------------------------------------------------------------------
// Write methods -- Todos
// ---------------------------------------------------------------------------

func DBCompleteTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return err
}

func DBDismissTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET status = 'dismissed', updated_at = ? WHERE id = ?`, now, id)
	return err
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
	return err
}

func DBDeferTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM cc_todos WHERE status = 'active'), updated_at = ? WHERE id = ?`,
		now, id)
	return err
}

func DBPromoteTodo(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_todos SET sort_order = (SELECT COALESCE(MIN(sort_order), 0) - 1 FROM cc_todos WHERE status = 'active'), updated_at = ? WHERE id = ?`,
		now, id)
	return err
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
	return err
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
	return err
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
	return err
}

// ---------------------------------------------------------------------------
// Write methods -- Threads
// ---------------------------------------------------------------------------

func DBPauseThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'paused', paused_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return err
}

func DBStartThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'active', paused_at = NULL, updated_at = ? WHERE id = ?`,
		now, id)
	return err
}

func DBCloseThread(db *sql.DB, id string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`UPDATE cc_threads SET status = 'completed', completed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id)
	return err
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
	return err
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
// Bookmarks
// ---------------------------------------------------------------------------

func DBLoadBookmarks(db *sql.DB) ([]Session, error) {
	rows, err := db.Query(`SELECT session_id, project, repo, branch, label, summary, created_at
		FROM cc_bookmarks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sid, createdStr string
		var project, repo, branch, label, summary sql.NullString
		if err := rows.Scan(&sid, &project, &repo, &branch, &label, &summary, &createdStr); err != nil {
			return nil, err
		}
		sessions = append(sessions, Session{
			SessionID: sid,
			Project:   project.String,
			Repo:      repo.String,
			Branch:    branch.String,
			Summary:   summary.String,
			Created:   ParseTime(createdStr),
			Type:      SessionBookmark,
		})
	}
	return sessions, rows.Err()
}

func DBInsertBookmark(db *sql.DB, b Session) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_bookmarks (session_id, project, repo, branch, label, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.SessionID, b.Project, b.Repo, b.Branch, "", b.Summary, FormatTime(b.Created))
	return err
}

func DBRemoveBookmark(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM cc_bookmarks WHERE session_id = ?`, sessionID)
	return err
}

// ---------------------------------------------------------------------------
// Learned Paths
// ---------------------------------------------------------------------------

func DBLoadPaths(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM cc_learned_paths ORDER BY added_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, rows.Err()
}

func DBAddPath(db *sql.DB, path string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO cc_learned_paths (path, added_at) VALUES (?, ?)`,
		path, FormatTime(time.Now()))
	return err
}

func DBRemovePath(db *sql.DB, path string) error {
	_, err := db.Exec(`DELETE FROM cc_learned_paths WHERE path = ?`, path)
	return err
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func ParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t.Local()
}

// DBIsEmpty returns true if no todos exist in the database yet.
func DBIsEmpty(db *sql.DB) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM cc_todos`).Scan(&count)
	return err != nil || count == 0
}
