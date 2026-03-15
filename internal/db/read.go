package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
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
	rows, err := db.Query(`SELECT id, COALESCE(display_id, 0), title, status, source, source_ref, context, detail,
		who_waiting, project_dir, launch_mode, due, effort, session_id, proposed_prompt, session_status,
		session_summary, COALESCE(triage_status, 'accepted'), created_at, completed_at
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
		var sourceRef, ctx, detail, who, projDir, launchMode, due, effort, sessionID, proposedPrompt, sessionStatus, sessionSummary sql.NullString
		var triageStatus string

		err := rows.Scan(&t.ID, &t.DisplayID, &t.Title, &t.Status, &t.Source,
			&sourceRef, &ctx, &detail, &who, &projDir, &launchMode, &due, &effort, &sessionID,
			&proposedPrompt, &sessionStatus, &sessionSummary, &triageStatus,
			&createdStr, &completedStr)
		if err != nil {
			return nil, err
		}

		t.SourceRef = sourceRef.String
		t.Context = ctx.String
		t.Detail = detail.String
		t.WhoWaiting = who.String
		t.ProjectDir = projDir.String
		t.LaunchMode = launchMode.String
		t.Due = due.String
		t.Effort = effort.String
		t.SessionID = sessionID.String
		t.ProposedPrompt = proposedPrompt.String
		t.SessionStatus = sessionStatus.String
		t.SessionSummary = sessionSummary.String
		t.TriageStatus = triageStatus
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
	if err := rows.Err(); err != nil {
		return cal, err
	}

	// Clamp multi-day events to their day boundaries so they sort and
	// display correctly (e.g. a 3-day event shows as starting at midnight
	// today, not at its original start time days ago).
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.Add(24 * time.Hour)
	tomorrowEnd := todayEnd.Add(24 * time.Hour)

	cal.Today = clampEventsToDayBounds(cal.Today, todayStart, todayEnd)
	cal.Tomorrow = clampEventsToDayBounds(cal.Tomorrow, todayEnd, tomorrowEnd)

	sortCalendarEvents(cal.Today)
	sortCalendarEvents(cal.Tomorrow)

	return cal, nil
}

// clampEventsToDayBounds adjusts multi-day events so their Start/End are
// clamped to the given day boundaries. This ensures correct sort order and
// display for events that span multiple days.
func clampEventsToDayBounds(events []CalendarEvent, dayStart, dayEnd time.Time) []CalendarEvent {
	for i := range events {
		if events[i].AllDay {
			continue
		}
		wasClamped := events[i].Start.Before(dayStart)
		if wasClamped {
			events[i].Start = dayStart
		}
		if events[i].End.After(dayEnd) {
			events[i].End = dayEnd
		}
		// Events that span the entire day after clamping are effectively all-day.
		// This catches Exchange/Outlook-style "all-day" events that use DateTime
		// instead of Date, and multi-day events clamped to day boundaries.
		if wasClamped && events[i].End.Sub(events[i].Start) >= 12*time.Hour {
			events[i].AllDay = true
		}
	}
	return events
}

// sortCalendarEvents sorts events with all-day events first, then timed
// events by start time.
func sortCalendarEvents(events []CalendarEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].AllDay != events[j].AllDay {
			return events[i].AllDay // all-day events first
		}
		return events[i].Start.Before(events[j].Start)
	})
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
		if err := json.Unmarshal([]byte(rankedJSON.String), &s.RankedTodoIDs); err != nil {
			log.Printf("WARNING: corrupt ranked_todo_ids JSON: %v", err)
		}
	}
	if reasonsJSON.Valid {
		if err := json.Unmarshal([]byte(reasonsJSON.String), &s.Reasons); err != nil {
			log.Printf("WARNING: corrupt reasons JSON: %v", err)
		}
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
	rows, err := db.Query(`SELECT session_id, project, repo, branch, label, summary, created_at, worktree_path, source_repo
		FROM cc_bookmarks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sid, createdStr string
		var project, repo, branch, label, summary, worktreePath, sourceRepo sql.NullString
		if err := rows.Scan(&sid, &project, &repo, &branch, &label, &summary, &createdStr, &worktreePath, &sourceRepo); err != nil {
			return nil, err
		}
		sessions = append(sessions, Session{
			SessionID:    sid,
			Project:      project.String,
			Repo:         repo.String,
			Branch:       branch.String,
			Summary:      summary.String,
			Created:      ParseTime(createdStr),
			Type:         SessionBookmark,
			WorktreePath: worktreePath.String,
			SourceRepo:   sourceRepo.String,
		})
	}
	return sessions, rows.Err()
}

// DBLoadPaths loads all learned paths from the database.
func DBLoadPaths(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM cc_learned_paths ORDER BY sort_order ASC, added_at ASC`)
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

// DBLoadPathsFull loads all learned paths with full metadata.
func DBLoadPathsFull(d *sql.DB) ([]PathEntry, error) {
	rows, err := d.Query(`SELECT path, description, added_at, sort_order FROM cc_learned_paths ORDER BY sort_order ASC, added_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PathEntry
	for rows.Next() {
		var e PathEntry
		var addedAt string
		if err := rows.Scan(&e.Path, &e.Description, &addedAt, &e.SortOrder); err != nil {
			return nil, err
		}
		e.AddedAt = ParseTime(addedAt)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []PathEntry{}
	}
	return entries, rows.Err()
}

// DBLoadSourceSync loads the sync status for a given data source.
func DBLoadSourceSync(d *sql.DB, source string) (*SourceSync, error) {
	var ss SourceSync
	var lastSuccess, lastError sql.NullString
	var updatedAt string
	err := d.QueryRow(`SELECT source, last_success, last_error, updated_at FROM cc_source_sync WHERE source = ?`, source).
		Scan(&ss.Source, &lastSuccess, &lastError, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastSuccess.Valid && lastSuccess.String != "" {
		t := ParseTime(lastSuccess.String)
		ss.LastSuccess = &t
	}
	ss.LastError = lastError.String
	ss.UpdatedAt = ParseTime(updatedAt)
	return &ss, nil
}

// DBLoadAllSourceSync loads sync status for all tracked sources.
func DBLoadAllSourceSync(d *sql.DB) (map[string]*SourceSync, error) {
	rows, err := d.Query(`SELECT source, last_success, last_error, updated_at FROM cc_source_sync`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*SourceSync)
	for rows.Next() {
		var ss SourceSync
		var lastSuccess, lastError sql.NullString
		var updatedAt string
		if err := rows.Scan(&ss.Source, &lastSuccess, &lastError, &updatedAt); err != nil {
			return nil, err
		}
		if lastSuccess.Valid && lastSuccess.String != "" {
			t := ParseTime(lastSuccess.String)
			ss.LastSuccess = &t
		}
		ss.LastError = lastError.String
		ss.UpdatedAt = ParseTime(updatedAt)
		result[ss.Source] = &ss
	}
	return result, rows.Err()
}

// DBLoadTodoByDisplayID loads a single todo by its display_id.
// Returns nil, nil if no todo with that display_id exists.
func DBLoadTodoByDisplayID(db *sql.DB, displayID int) (*Todo, error) {
	var t Todo
	var createdStr string
	var completedStr sql.NullString
	var sourceRef, ctx, detail, who, projDir, launchMode, due, effort, sessionID, proposedPrompt, sessionStatus, sessionSummary sql.NullString

	err := db.QueryRow(`SELECT id, COALESCE(display_id, 0), title, status, source, source_ref, context, detail,
		who_waiting, project_dir, launch_mode, due, effort, session_id, proposed_prompt, session_status,
		session_summary, created_at, completed_at
		FROM cc_todos WHERE display_id = ?`, displayID).
		Scan(&t.ID, &t.DisplayID, &t.Title, &t.Status, &t.Source,
			&sourceRef, &ctx, &detail, &who, &projDir, &launchMode, &due, &effort, &sessionID,
			&proposedPrompt, &sessionStatus, &sessionSummary,
			&createdStr, &completedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.SourceRef = sourceRef.String
	t.Context = ctx.String
	t.Detail = detail.String
	t.WhoWaiting = who.String
	t.ProjectDir = projDir.String
	t.LaunchMode = launchMode.String
	t.Due = due.String
	t.Effort = effort.String
	t.SessionID = sessionID.String
	t.ProposedPrompt = proposedPrompt.String
	t.SessionStatus = sessionStatus.String
	t.SessionSummary = sessionSummary.String
	t.CreatedAt = ParseTime(createdStr)
	if completedStr.Valid {
		ct := ParseTime(completedStr.String)
		t.CompletedAt = &ct
	}
	return &t, nil
}

// DBIsEmpty returns true if no todos exist in the database yet.
func DBIsEmpty(db *sql.DB) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM cc_todos`).Scan(&count)
	return err != nil || count == 0
}
