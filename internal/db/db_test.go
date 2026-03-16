package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DB tests (from original db_test.go)
// ---------------------------------------------------------------------------

func TestOpenDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM cc_todos").Scan(&count); err != nil {
		t.Fatalf("query cc_todos: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 todos, got %d", count)
	}
}

func TestTodoRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	todo := Todo{
		ID:        "abcd1234",
		Title:     "Test todo",
		Status:    "active",
		Source:    "manual",
		CreatedAt: time.Now(),
	}
	if err := DBInsertTodo(db, todo); err != nil {
		t.Fatalf("insert: %v", err)
	}

	cc, err := LoadCommandCenterFromDB(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cc.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(cc.Todos))
	}
	if cc.Todos[0].ID != "abcd1234" {
		t.Fatalf("expected id abcd1234, got %s", cc.Todos[0].ID)
	}
	if cc.Todos[0].Title != "Test todo" {
		t.Fatalf("expected title 'Test todo', got %s", cc.Todos[0].Title)
	}
}

func TestDBCompleteTodo(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	todo := Todo{
		ID: "abcd1234", Title: "Test", Status: "active", Source: "manual",
		CreatedAt: time.Now(),
	}
	DBInsertTodo(db, todo)
	DBCompleteTodo(db, "abcd1234")

	cc, _ := LoadCommandCenterFromDB(db)
	if cc.Todos[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", cc.Todos[0].Status)
	}
	if cc.Todos[0].CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestDBDismissTodo(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	todo := Todo{
		ID: "abcd1234", Title: "Test", Status: "active", Source: "manual",
		CreatedAt: time.Now(),
	}
	DBInsertTodo(db, todo)
	DBDismissTodo(db, "abcd1234")

	cc, _ := LoadCommandCenterFromDB(db)
	if cc.Todos[0].Status != "dismissed" {
		t.Fatalf("expected dismissed, got %s", cc.Todos[0].Status)
	}
}

func TestDBDeferTodo(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	DBInsertTodo(db, Todo{ID: "aaa", Title: "First", Status: "active", Source: "manual", CreatedAt: time.Now()})
	DBInsertTodo(db, Todo{ID: "bbb", Title: "Second", Status: "active", Source: "manual", CreatedAt: time.Now()})

	DBDeferTodo(db, "aaa")

	cc, _ := LoadCommandCenterFromDB(db)
	if len(cc.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(cc.Todos))
	}
	if cc.Todos[0].ID != "bbb" {
		t.Fatalf("expected bbb first after defer, got %s", cc.Todos[0].ID)
	}
	if cc.Todos[1].ID != "aaa" {
		t.Fatalf("expected aaa second after defer, got %s", cc.Todos[1].ID)
	}
}

func TestThreadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	thread := Thread{
		ID: "th01", Type: "pr", Title: "Fix bug", URL: "https://github.com/test/1",
		Status: "active", CreatedAt: time.Now(),
	}
	DBInsertThread(db, thread)

	cc, _ := LoadCommandCenterFromDB(db)
	if len(cc.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(cc.Threads))
	}
	if cc.Threads[0].Title != "Fix bug" {
		t.Fatalf("expected 'Fix bug', got %s", cc.Threads[0].Title)
	}

	DBPauseThread(db, "th01")
	cc, _ = LoadCommandCenterFromDB(db)
	if cc.Threads[0].Status != "paused" {
		t.Fatalf("expected paused, got %s", cc.Threads[0].Status)
	}

	DBStartThread(db, "th01")
	cc, _ = LoadCommandCenterFromDB(db)
	if cc.Threads[0].Status != "active" {
		t.Fatalf("expected active, got %s", cc.Threads[0].Status)
	}

	DBCloseThread(db, "th01")
	cc, _ = LoadCommandCenterFromDB(db)
	if cc.Threads[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", cc.Threads[0].Status)
	}
}

func TestPathRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	DBAddPath(db, "/home/user/project-a")
	DBAddPath(db, "/home/user/project-b")
	DBAddPath(db, "/home/user/project-a") // duplicate, should be ignored

	paths, err := DBLoadPaths(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}

	DBRemovePath(db, "/home/user/project-a")
	paths, _ = DBLoadPaths(db)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after remove, got %d", len(paths))
	}
}

func TestPathDescription(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	DBAddPath(db, "/home/user/project-a")
	DBAddPath(db, "/home/user/project-b")

	// Initially no descriptions
	paths, err := DBLoadPathsFull(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0].Description != "" {
		t.Fatalf("expected empty description, got %q", paths[0].Description)
	}

	// Set description
	if err := DBUpdatePathDescription(db, "/home/user/project-a", "Go TUI dashboard"); err != nil {
		t.Fatalf("set description: %v", err)
	}

	paths, _ = DBLoadPathsFull(db)
	if paths[0].Path != "/home/user/project-a" {
		t.Fatalf("expected project-a first, got %s", paths[0].Path)
	}
	if paths[0].Description != "Go TUI dashboard" {
		t.Fatalf("expected 'Go TUI dashboard', got %q", paths[0].Description)
	}
	if paths[1].Description != "" {
		t.Fatalf("expected empty description for project-b, got %q", paths[1].Description)
	}

	// Update description for nonexistent path
	err = DBUpdatePathDescription(db, "/nonexistent", "desc")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestBookmarkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	bm := Session{
		SessionID: "sess-123",
		Project:   "/home/user/proj",
		Repo:      "proj",
		Branch:    "main",
		Summary:   "Working on feature",
		Created:   time.Now(),
		Type:      SessionBookmark,
	}
	DBInsertBookmark(db, bm, "")

	bookmarks, err := DBLoadBookmarks(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	if bookmarks[0].SessionID != "sess-123" {
		t.Fatalf("expected sess-123, got %s", bookmarks[0].SessionID)
	}

	DBRemoveBookmark(db, "sess-123")
	bookmarks, _ = DBLoadBookmarks(db)
	if len(bookmarks) != 0 {
		t.Fatalf("expected 0 bookmarks after remove, got %d", len(bookmarks))
	}
}

func TestMigrateFromJSON(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	ccJSON := `{
		"generated_at": "2026-03-05T10:00:00Z",
		"calendar": {"today": [], "tomorrow": []},
		"todos": [
			{"id": "t1", "title": "First todo", "status": "active", "source": "granola", "source_ref": "https://meeting/1", "created_at": "2026-03-05T09:00:00Z"},
			{"id": "t2", "title": "Dismissed one", "status": "dismissed", "source": "slack", "source_ref": "https://slack/1", "created_at": "2026-03-05T08:00:00Z"}
		],
		"threads": [
			{"id": "th1", "type": "pr", "title": "Fix bug", "status": "active", "url": "https://github.com/1", "created_at": "2026-03-05T07:00:00Z"}
		],
		"suggestions": {"focus": "Do the first thing", "ranked_todo_ids": ["t1"], "reasons": {"t1": "urgent"}},
		"pending_actions": []
	}`
	ccPath := filepath.Join(dir, "command-center.json")
	os.WriteFile(ccPath, []byte(ccJSON), 0o644)

	bmJSON := `[{"session_id": "bm1", "project": "/proj", "repo": "proj", "branch": "main", "label": "", "summary": "test", "created": "2026-03-05T06:00:00Z"}]`
	bmPath := filepath.Join(dir, "bookmarks.json")
	os.WriteFile(bmPath, []byte(bmJSON), 0o644)

	pathsPath := filepath.Join(dir, "paths.txt")
	os.WriteFile(pathsPath, []byte("/path/one\n/path/two\n"), 0o644)

	if err := MigrateFromJSON(db, ccPath, bmPath, pathsPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cc, err := LoadCommandCenterFromDB(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cc.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(cc.Todos))
	}
	if cc.Todos[0].ID != "t1" {
		t.Fatalf("expected t1 first (by sort_order), got %s", cc.Todos[0].ID)
	}
	if cc.Todos[1].Status != "dismissed" {
		t.Fatalf("expected dismissed status preserved, got %s", cc.Todos[1].Status)
	}
	if len(cc.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(cc.Threads))
	}
	if cc.Suggestions.Focus != "Do the first thing" {
		t.Fatalf("expected suggestions focus, got %s", cc.Suggestions.Focus)
	}

	bookmarks, _ := DBLoadBookmarks(db)
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	paths, _ := DBLoadPaths(db)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}

	// Run migration again -- should be idempotent (DB is no longer empty)
	if err := MigrateFromJSON(db, ccPath, bmPath, pathsPath); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	cc, _ = LoadCommandCenterFromDB(db)
	if len(cc.Todos) != 2 {
		t.Fatalf("idempotent check: expected 2 todos, got %d", len(cc.Todos))
	}
}

func TestDBSaveRefreshResult(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	cc := &CommandCenter{
		GeneratedAt: now,
		Calendar: CalendarData{
			Today: []CalendarEvent{
				{Title: "Standup", Start: now, End: now.Add(30 * time.Minute), CalendarID: "primary"},
			},
			Tomorrow: []CalendarEvent{
				{Title: "Review", Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour), AllDay: false},
			},
		},
		Todos: []Todo{
			{ID: "t1", Title: "Fix bug", Status: "active", Source: "github", CreatedAt: now},
			{ID: "t2", Title: "Done task", Status: "completed", Source: "manual", CreatedAt: now, CompletedAt: &now},
		},
		Threads: []Thread{
			{ID: "th1", Type: "pr", Title: "PR #42", URL: "https://github.com/repo/42", Status: "active", CreatedAt: now},
		},
		Suggestions: Suggestions{
			Focus:        "Fix the bug first",
			RankedTodoIDs: []string{"t1", "t2"},
			Reasons:      map[string]string{"t1": "blocking release"},
		},
		PendingActions: []PendingAction{
			{Type: "book", TodoID: "t1", DurationMinutes: 60, RequestedAt: now},
		},
	}

	if err := DBSaveRefreshResult(db, cc); err != nil {
		t.Fatalf("DBSaveRefreshResult: %v", err)
	}

	loaded, err := LoadCommandCenterFromDB(db)
	if err != nil {
		t.Fatalf("LoadCommandCenterFromDB: %v", err)
	}

	// Todos
	if len(loaded.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(loaded.Todos))
	}
	if loaded.Todos[0].ID != "t1" || loaded.Todos[1].ID != "t2" {
		t.Fatalf("todo order mismatch: %s, %s", loaded.Todos[0].ID, loaded.Todos[1].ID)
	}
	if loaded.Todos[1].CompletedAt == nil {
		t.Fatal("expected completed_at to be preserved")
	}

	// Threads
	if len(loaded.Threads) != 1 || loaded.Threads[0].URL != "https://github.com/repo/42" {
		t.Fatalf("thread mismatch")
	}

	// Calendar
	if len(loaded.Calendar.Today) != 1 || loaded.Calendar.Today[0].Title != "Standup" {
		t.Fatalf("calendar today mismatch")
	}
	if len(loaded.Calendar.Tomorrow) != 1 {
		t.Fatalf("expected 1 tomorrow event, got %d", len(loaded.Calendar.Tomorrow))
	}

	// Suggestions
	if loaded.Suggestions.Focus != "Fix the bug first" {
		t.Fatalf("suggestions focus mismatch: %q", loaded.Suggestions.Focus)
	}
	if len(loaded.Suggestions.RankedTodoIDs) != 2 {
		t.Fatalf("expected 2 ranked IDs, got %d", len(loaded.Suggestions.RankedTodoIDs))
	}

	// Pending actions
	if len(loaded.PendingActions) != 1 || loaded.PendingActions[0].TodoID != "t1" {
		t.Fatalf("pending actions mismatch")
	}

	// Overwrite with new data — verify replace-all behavior
	cc.Todos = []Todo{{ID: "t3", Title: "New only", Status: "active", Source: "manual", CreatedAt: now}}
	cc.PendingActions = nil
	if err := DBSaveRefreshResult(db, cc); err != nil {
		t.Fatalf("second save: %v", err)
	}
	loaded, _ = LoadCommandCenterFromDB(db)
	if len(loaded.Todos) != 1 || loaded.Todos[0].ID != "t3" {
		t.Fatalf("expected only t3 after overwrite, got %d todos", len(loaded.Todos))
	}
	if len(loaded.PendingActions) != 0 {
		t.Fatalf("expected 0 pending actions after overwrite, got %d", len(loaded.PendingActions))
	}
}

func TestDBLoadTodoByDisplayIDIncludesTriageStatus(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	todo := Todo{
		ID:           "triage-test",
		Title:        "Test triage status",
		Status:       "active",
		Source:       "manual",
		TriageStatus: "new",
		CreatedAt:    time.Now(),
	}
	if err := DBInsertTodo(db, todo); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Load by display_id (assigned automatically by DBInsertTodo)
	loaded, err := DBLoadTodoByDisplayID(db, 1)
	if err != nil {
		t.Fatalf("DBLoadTodoByDisplayID: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil todo")
	}
	if loaded.TriageStatus != "new" {
		t.Errorf("expected TriageStatus 'new', got %q", loaded.TriageStatus)
	}

	// Also test the COALESCE default: insert a todo with empty triage_status
	// which should default to "accepted"
	todo2 := Todo{
		ID:        "triage-default",
		Title:     "Default triage",
		Status:    "active",
		Source:    "manual",
		CreatedAt: time.Now(),
	}
	if err := DBInsertTodo(db, todo2); err != nil {
		t.Fatalf("insert: %v", err)
	}
	loaded2, err := DBLoadTodoByDisplayID(db, 2)
	if err != nil {
		t.Fatalf("DBLoadTodoByDisplayID: %v", err)
	}
	if loaded2 == nil {
		t.Fatal("expected non-nil todo")
	}
	if loaded2.TriageStatus != "accepted" {
		t.Errorf("expected default TriageStatus 'accepted', got %q", loaded2.TriageStatus)
	}
}

func TestDBSaveRefreshResultPreservesLaunchMode(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	cc := &CommandCenter{
		GeneratedAt: now,
		Todos: []Todo{
			{ID: "t1", Title: "Todo with launch mode", Status: "active", Source: "manual", LaunchMode: "worktree", CreatedAt: now},
			{ID: "t2", Title: "Todo without launch mode", Status: "active", Source: "manual", CreatedAt: now},
		},
	}

	if err := DBSaveRefreshResult(db, cc); err != nil {
		t.Fatalf("DBSaveRefreshResult: %v", err)
	}

	loaded, err := LoadCommandCenterFromDB(db)
	if err != nil {
		t.Fatalf("LoadCommandCenterFromDB: %v", err)
	}

	if len(loaded.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(loaded.Todos))
	}
	if loaded.Todos[0].LaunchMode != "worktree" {
		t.Errorf("expected LaunchMode 'worktree', got %q", loaded.Todos[0].LaunchMode)
	}
	if loaded.Todos[1].LaunchMode != "" {
		t.Errorf("expected empty LaunchMode, got %q", loaded.Todos[1].LaunchMode)
	}
}

func TestDBSaveSuggestions(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	s := Suggestions{
		Focus:         "Ship the feature",
		RankedTodoIDs: []string{"a", "b"},
		Reasons:       map[string]string{"a": "deadline tomorrow"},
	}
	if err := DBSaveSuggestions(db, s); err != nil {
		t.Fatalf("DBSaveSuggestions: %v", err)
	}

	cc, _ := LoadCommandCenterFromDB(db)
	if cc.Suggestions.Focus != "Ship the feature" {
		t.Fatalf("expected focus, got %q", cc.Suggestions.Focus)
	}
	if len(cc.Suggestions.Reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d", len(cc.Suggestions.Reasons))
	}
}

func TestDBSetMeta(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if err := DBSetMeta(db, "version", "2.0"); err != nil {
		t.Fatalf("DBSetMeta: %v", err)
	}

	var value string
	db.QueryRow(`SELECT value FROM cc_meta WHERE key = ?`, "version").Scan(&value)
	if value != "2.0" {
		t.Fatalf("expected '2.0', got %q", value)
	}

	// Upsert
	DBSetMeta(db, "version", "3.0")
	db.QueryRow(`SELECT value FROM cc_meta WHERE key = ?`, "version").Scan(&value)
	if value != "3.0" {
		t.Fatalf("expected '3.0' after upsert, got %q", value)
	}
}

func TestDBClearPendingActions(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	now := time.Now()
	cc := &CommandCenter{
		PendingActions: []PendingAction{
			{Type: "book", TodoID: "t1", DurationMinutes: 30, RequestedAt: now},
			{Type: "book", TodoID: "t2", DurationMinutes: 60, RequestedAt: now},
		},
	}
	DBSaveRefreshResult(db, cc)

	if err := DBClearPendingActions(db); err != nil {
		t.Fatalf("DBClearPendingActions: %v", err)
	}

	loaded, _ := LoadCommandCenterFromDB(db)
	if len(loaded.PendingActions) != 0 {
		t.Fatalf("expected 0 pending actions, got %d", len(loaded.PendingActions))
	}
}

func TestDBIsEmpty(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if !DBIsEmpty(db) {
		t.Fatal("expected empty DB")
	}

	DBInsertTodo(db, Todo{ID: "x", Title: "test", Status: "active", Source: "manual", CreatedAt: time.Now()})
	if DBIsEmpty(db) {
		t.Fatal("expected non-empty DB")
	}
}

// ---------------------------------------------------------------------------
// CommandCenter type tests (from original command_center_test.go)
// ---------------------------------------------------------------------------

func TestCompleteTodo(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Title: "First", Status: "active"},
			{ID: "2", Title: "Second", Status: "active"},
		},
	}

	cc.CompleteTodo("1")

	if cc.Todos[0].Status != "completed" {
		t.Errorf("expected completed, got %s", cc.Todos[0].Status)
	}
	if cc.Todos[0].CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if cc.Todos[1].Status != "active" {
		t.Error("second todo should still be active")
	}
}

func TestRemoveTodo(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Title: "First", Status: "active"},
			{ID: "2", Title: "Second", Status: "active"},
			{ID: "3", Title: "Third", Status: "active"},
		},
	}

	cc.RemoveTodo("2")

	if len(cc.Todos) != 3 {
		t.Fatalf("expected 3 todos (soft-delete), got %d", len(cc.Todos))
	}
	if cc.Todos[1].Status != "dismissed" {
		t.Errorf("expected todo 2 to be dismissed, got %s", cc.Todos[1].Status)
	}
	active := cc.ActiveTodos()
	if len(active) != 2 {
		t.Fatalf("expected 2 active todos, got %d", len(active))
	}
	if active[0].ID != "1" || active[1].ID != "3" {
		t.Error("expected active todos 1 and 3")
	}
}

func TestAddTodo(t *testing.T) {
	cc := &CommandCenter{}
	todo := cc.AddTodo("New task")

	if len(cc.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(cc.Todos))
	}
	if todo.Title != "New task" {
		t.Errorf("expected title 'New task', got %q", todo.Title)
	}
	if todo.Source != "manual" {
		t.Errorf("expected source 'manual', got %q", todo.Source)
	}
	if todo.Status != "active" {
		t.Errorf("expected status 'active', got %q", todo.Status)
	}
	if todo.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestDeferTodo(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Title: "First", Status: "active"},
			{ID: "2", Title: "Second", Status: "active"},
			{ID: "3", Title: "Third", Status: "active"},
			{ID: "done", Title: "Done", Status: "completed"},
		},
	}

	cc.DeferTodo("1")

	active := cc.ActiveTodos()
	if len(active) != 3 {
		t.Fatalf("expected 3 active, got %d", len(active))
	}
	if active[0].ID != "2" {
		t.Errorf("expected '2' first, got %q", active[0].ID)
	}
	if active[2].ID != "1" {
		t.Errorf("expected '1' last, got %q", active[2].ID)
	}
}

func TestPauseThread(t *testing.T) {
	cc := &CommandCenter{
		Threads: []Thread{
			{ID: "t1", Status: "active"},
		},
	}
	cc.PauseThread("t1")

	if cc.Threads[0].Status != "paused" {
		t.Errorf("expected paused, got %s", cc.Threads[0].Status)
	}
	if cc.Threads[0].PausedAt == nil {
		t.Error("expected PausedAt to be set")
	}
}

func TestStartThread(t *testing.T) {
	now := time.Now()
	cc := &CommandCenter{
		Threads: []Thread{
			{ID: "t1", Status: "paused", PausedAt: &now},
		},
	}
	cc.StartThread("t1")

	if cc.Threads[0].Status != "active" {
		t.Errorf("expected active, got %s", cc.Threads[0].Status)
	}
	if cc.Threads[0].PausedAt != nil {
		t.Error("expected PausedAt to be nil")
	}
}

func TestCloseThread(t *testing.T) {
	cc := &CommandCenter{
		Threads: []Thread{
			{ID: "t1", Status: "active"},
		},
	}
	cc.CloseThread("t1")

	if cc.Threads[0].Status != "completed" {
		t.Errorf("expected completed, got %s", cc.Threads[0].Status)
	}
	if cc.Threads[0].CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestAddThread(t *testing.T) {
	cc := &CommandCenter{}
	thread := cc.AddThread("New thread", "pr")

	if len(cc.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(cc.Threads))
	}
	if thread.Title != "New thread" {
		t.Errorf("expected title 'New thread', got %q", thread.Title)
	}
	if thread.Type != "pr" {
		t.Errorf("expected type 'pr', got %q", thread.Type)
	}
	if thread.Status != "active" {
		t.Errorf("expected status 'active', got %q", thread.Status)
	}
}

func TestActiveTodos(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Status: "active"},
			{ID: "2", Status: "completed"},
			{ID: "3", Status: "active"},
		},
	}
	active := cc.ActiveTodos()
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}
}

func TestActiveAndPausedThreads(t *testing.T) {
	now := time.Now()
	cc := &CommandCenter{
		Threads: []Thread{
			{ID: "1", Status: "active", CreatedAt: now},
			{ID: "2", Status: "paused", PausedAt: &now},
			{ID: "3", Status: "completed"},
			{ID: "4", Status: "active", CreatedAt: now.Add(-time.Hour)},
		},
	}

	active := cc.ActiveThreads()
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}
	if active[0].ID != "4" {
		t.Errorf("expected oldest first, got %s", active[0].ID)
	}

	paused := cc.PausedThreads()
	if len(paused) != 1 {
		t.Fatalf("expected 1 paused, got %d", len(paused))
	}
}

func TestDueUrgency(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	nextWeek := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	tests := []struct {
		due    string
		expect string
	}{
		{"", "none"},
		{yesterday, "overdue"},
		{today, "soon"},
		{tomorrow, "soon"},
		{nextWeek, "later"},
		{"bad-date", "none"},
	}

	for _, tc := range tests {
		got := DueUrgency(tc.due)
		if got != tc.expect {
			t.Errorf("DueUrgency(%q) = %q, want %q", tc.due, got, tc.expect)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		t      time.Time
		expect string
	}{
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-90 * time.Minute), "1h ago"},
		{now.Add(-48 * time.Hour), "2d ago"},
	}

	for _, tc := range tests {
		got := RelativeTime(tc.t)
		if got != tc.expect {
			t.Errorf("RelativeTime(%v) = %q, want %q", tc.t, got, tc.expect)
		}
	}
}

func TestAddPendingBooking(t *testing.T) {
	cc := &CommandCenter{}
	cc.AddPendingBooking("todo-1", 60)

	if len(cc.PendingActions) != 1 {
		t.Fatalf("expected 1 pending action, got %d", len(cc.PendingActions))
	}
	if cc.PendingActions[0].TodoID != "todo-1" {
		t.Errorf("expected todo-1, got %s", cc.PendingActions[0].TodoID)
	}
	if cc.PendingActions[0].DurationMinutes != 60 {
		t.Errorf("expected 60 min, got %d", cc.PendingActions[0].DurationMinutes)
	}
}

func TestFindConflicts(t *testing.T) {
	now := time.Now().Add(time.Hour)
	cal := &CalendarData{
		Today: []CalendarEvent{
			{Title: "Meeting A", Start: now, End: now.Add(time.Hour)},
			{Title: "Meeting B", Start: now.Add(30 * time.Minute), End: now.Add(90 * time.Minute)},
			{Title: "Meeting C", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		},
		Tomorrow: []CalendarEvent{
			{Title: "Morning", Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour)},
			{Title: "Overlap", Start: now.Add(24*time.Hour + 30*time.Minute), End: now.Add(25*time.Hour + 30*time.Minute)},
		},
	}

	conflicts := cal.FindConflicts()
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
	if conflicts[0].Day != "today" {
		t.Errorf("expected first conflict on today, got %s", conflicts[0].Day)
	}
	if conflicts[1].Day != "tomorrow" {
		t.Errorf("expected second conflict on tomorrow, got %s", conflicts[1].Day)
	}
	if conflicts[0].EventA != "Meeting A" || conflicts[0].EventB != "Meeting B" {
		t.Error("expected conflict between Meeting A and Meeting B")
	}
}

func TestFindConflicts_NoOverlap(t *testing.T) {
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.Local)
	cal := &CalendarData{
		Today: []CalendarEvent{
			{Title: "A", Start: now, End: now.Add(time.Hour)},
			{Title: "B", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		},
	}
	conflicts := cal.FindConflicts()
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}

// ---------------------------------------------------------------------------
// Data tests (from original data_test.go)
// ---------------------------------------------------------------------------

func TestLoadPaths_FileNotExist(t *testing.T) {
	paths, err := LoadPaths("/nonexistent/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected empty slice, got %v", paths)
	}
}

func TestLoadPaths_WithContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "paths.txt")
	os.WriteFile(file, []byte("/usr/local\n/home/user\n"), 0o644)

	paths, err := LoadPaths(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "/usr/local" || paths[1] != "/home/user" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestLoadPaths_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "paths.txt")
	os.WriteFile(file, []byte("/a\n\n  \n/b\n"), 0o644)

	paths, err := LoadPaths(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/a" || paths[1] != "/b" {
		t.Fatalf("unexpected paths: %v", paths)
	}
}

func TestSavePaths_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "deep", "paths.txt")

	err := SavePaths(file, []string{"/foo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "/foo\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestSavePaths_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "paths.txt")

	original := []string{"/alpha", "/beta", "/gamma"}
	if err := SavePaths(file, original); err != nil {
		t.Fatalf("save error: %v", err)
	}
	loaded, err := LoadPaths(file)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if len(loaded) != len(original) {
		t.Fatalf("expected %d, got %d", len(original), len(loaded))
	}
	for i := range original {
		if loaded[i] != original[i] {
			t.Fatalf("mismatch at %d: %q != %q", i, loaded[i], original[i])
		}
	}
}

func TestAddPath_NoDuplicates(t *testing.T) {
	paths := []string{"/a", "/b"}
	result := AddPath(paths, "/a")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestAddPath_AppendsNew(t *testing.T) {
	paths := []string{"/a", "/b"}
	result := AddPath(paths, "/c")
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[2] != "/c" {
		t.Fatalf("expected /c at end, got %q", result[2])
	}
}

func TestRemovePath_Existing(t *testing.T) {
	paths := []string{"/a", "/b", "/c"}
	result := RemovePath(paths, "/b")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	for _, p := range result {
		if p == "/b" {
			t.Fatal("/b should have been removed")
		}
	}
}

func TestRemovePath_NotFound(t *testing.T) {
	paths := []string{"/a", "/b"}
	result := RemovePath(paths, "/z")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestParseSessionFile_Valid(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "session1.md")
	content := `---
project: /home/user/projects/myapp
repo: sherlock
branch: main
created: 2026-03-04T10:21:20
summary: Investigation templates
---

Some body text here.
`
	os.WriteFile(file, []byte(content), 0o644)

	s, err := ParseSessionFile(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Filename != "session1.md" {
		t.Fatalf("unexpected filename: %q", s.Filename)
	}
	if s.Project != "/home/user/projects/myapp" {
		t.Fatalf("unexpected project: %q", s.Project)
	}
	if s.Repo != "sherlock" {
		t.Fatalf("unexpected repo: %q", s.Repo)
	}
	if s.Branch != "main" {
		t.Fatalf("unexpected branch: %q", s.Branch)
	}
	if s.Summary != "Investigation templates" {
		t.Fatalf("unexpected summary: %q", s.Summary)
	}
	expected := time.Date(2026, 3, 4, 10, 21, 20, 0, time.UTC)
	if !s.Created.Equal(expected) {
		t.Fatalf("unexpected created: %v", s.Created)
	}
}

func TestParseSessionFile_MissingFields(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sparse.md")
	content := `---
project: /some/path
---
`
	os.WriteFile(file, []byte(content), 0o644)

	s, err := ParseSessionFile(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Project != "/some/path" {
		t.Fatalf("unexpected project: %q", s.Project)
	}
	if s.Repo != "" || s.Branch != "" || s.Summary != "" {
		t.Fatal("expected empty fields for missing keys")
	}
	if !s.Created.IsZero() {
		t.Fatal("expected zero time for missing created")
	}
}

func TestParseSessionFile_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nofm.md")
	os.WriteFile(file, []byte("just some text\n"), 0o644)

	_, err := ParseSessionFile(file)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestLoadWinddownSessions_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	for i, name := range []string{"a.md", "b.md"} {
		content := "---\nproject: /proj" + string(rune('A'+i)) + "\nrepo: repo\nbranch: main\ncreated: 2026-01-01T00:00:00\nsummary: test\n---\n"
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644)
	os.MkdirAll(filepath.Join(dir, "resumed"), 0o755)
	os.WriteFile(filepath.Join(dir, "resumed", "c.md"), []byte("---\nproject: /skip\n---\n"), 0o644)

	sessions, err := LoadWinddownSessions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestLoadWinddownSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sessions, err := LoadWinddownSessions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadWinddownSessions_DirNotExist(t *testing.T) {
	sessions, err := LoadWinddownSessions("/nonexistent/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadBookmarks_FileNotExist(t *testing.T) {
	sessions, err := LoadBookmarks("/nonexistent/bookmarks.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestLoadBookmarks_Valid(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bookmarks.json")
	content := `[
  {
    "session_id": "abc-123",
    "project": "/home/user/proj",
    "repo": "proj",
    "branch": "main",
    "label": "test bookmark",
    "summary": "Working on tests",
    "created": "2026-03-04T10:00:00Z"
  }
]`
	os.WriteFile(file, []byte(content), 0o644)

	sessions, err := LoadBookmarks(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.SessionID != "abc-123" {
		t.Fatalf("unexpected session ID: %q", s.SessionID)
	}
	if s.Type != SessionBookmark {
		t.Fatalf("expected SessionBookmark type, got %d", s.Type)
	}
	if s.Summary != "Working on tests" {
		t.Fatalf("unexpected summary: %q", s.Summary)
	}
}

func TestLoadAllSessions_MergedAndSorted(t *testing.T) {
	sessDir := t.TempDir()
	content := "---\nproject: /proj\nrepo: proj\nbranch: main\ncreated: 2026-03-01T00:00:00\nsummary: old winddown\n---\n"
	os.WriteFile(filepath.Join(sessDir, "old.md"), []byte(content), 0o644)

	bmDir := t.TempDir()
	bmFile := filepath.Join(bmDir, "bookmarks.json")
	bmContent := `[{"session_id":"new-uuid","project":"/proj","repo":"proj","branch":"feat","label":"new","summary":"new bookmark","created":"2026-03-05T00:00:00Z"}]`
	os.WriteFile(bmFile, []byte(bmContent), 0o644)

	sessions, err := LoadAllSessions(sessDir, bmFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Type != SessionBookmark {
		t.Fatal("expected bookmark first (newest)")
	}
	if sessions[1].Type != SessionWinddown {
		t.Fatal("expected winddown second (oldest)")
	}
}

func TestRemoveBookmarkFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bookmarks.json")
	content := `[{"session_id":"keep","project":"/a","repo":"a","branch":"main","label":"a","summary":"a","created":"2026-01-01T00:00:00Z"},{"session_id":"remove","project":"/b","repo":"b","branch":"main","label":"b","summary":"b","created":"2026-01-02T00:00:00Z"}]`
	os.WriteFile(file, []byte(content), 0o644)

	err := RemoveBookmark(file, "remove")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sessions, _ := LoadBookmarks(file)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "keep" {
		t.Fatalf("wrong session kept: %q", sessions[0].SessionID)
	}
}
