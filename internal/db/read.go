package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

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

// DBLoadBookmarks loads all bookmarked sessions from the database.
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

// DBLoadPaths loads all learned paths from the database.
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

// DBIsEmpty returns true if no todos exist in the database yet.
func DBIsEmpty(db *sql.DB) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM cc_todos`).Scan(&count)
	return err != nil || count == 0
}
