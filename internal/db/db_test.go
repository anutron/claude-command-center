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
		Status:    StatusBacklog,
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
		ID: "abcd1234", Title: "Test", Status: StatusBacklog, Source: "manual",
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
		ID: "abcd1234", Title: "Test", Status: StatusBacklog, Source: "manual",
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

	DBInsertTodo(db, Todo{ID: "aaa", Title: "First", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})
	DBInsertTodo(db, Todo{ID: "bbb", Title: "Second", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})

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
			{ID: "t1", Title: "Fix bug", Status: StatusBacklog, Source: "github", CreatedAt: now},
			{ID: "t2", Title: "Done task", Status: "completed", Source: "manual", CreatedAt: now, CompletedAt: &now},
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
	cc.Todos = []Todo{{ID: "t3", Title: "New only", Status: StatusBacklog, Source: "manual", CreatedAt: now}}
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

func TestDBLoadTodoByDisplayIDIncludesStatus(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	todo := Todo{
		ID:        "status-test",
		Title:     "Test status",
		Status:    StatusNew,
		Source:    "slack",
		CreatedAt: time.Now(),
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
	if loaded.Status != StatusNew {
		t.Errorf("expected Status %q, got %q", StatusNew, loaded.Status)
	}

	// Insert a backlog todo
	todo2 := Todo{
		ID:        "backlog-test",
		Title:     "Backlog todo",
		Status:    StatusBacklog,
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
	if loaded2.Status != StatusBacklog {
		t.Errorf("expected Status %q, got %q", StatusBacklog, loaded2.Status)
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
			{ID: "t1", Title: "Todo with launch mode", Status: StatusBacklog, Source: "manual", LaunchMode: "worktree", CreatedAt: now},
			{ID: "t2", Title: "Todo without launch mode", Status: StatusBacklog, Source: "manual", CreatedAt: now},
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

	DBInsertTodo(db, Todo{ID: "x", Title: "test", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})
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
			{ID: "1", Title: "First", Status: StatusBacklog},
			{ID: "2", Title: "Second", Status: StatusBacklog},
		},
	}

	cc.CompleteTodo("1")

	if cc.Todos[0].Status != "completed" {
		t.Errorf("expected completed, got %s", cc.Todos[0].Status)
	}
	if cc.Todos[0].CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if cc.Todos[1].Status != StatusBacklog {
		t.Error("second todo should still be backlog")
	}
}

func TestRemoveTodo(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Title: "First", Status: StatusBacklog},
			{ID: "2", Title: "Second", Status: StatusBacklog},
			{ID: "3", Title: "Third", Status: StatusBacklog},
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
	if todo.Status != StatusBacklog {
		t.Errorf("expected status %q, got %q", StatusBacklog, todo.Status)
	}
	if todo.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestDeferTodo(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Title: "First", Status: StatusBacklog},
			{ID: "2", Title: "Second", Status: StatusBacklog},
			{ID: "3", Title: "Third", Status: StatusBacklog},
			{ID: "done", Title: "Done", Status: StatusCompleted},
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

func TestDeferTodoWithInterleavedTerminal(t *testing.T) {
	// Completed items retain their original sort_order, so they can
	// appear before active items in cc.Todos. DeferTodo must place
	// the item after the last active item, not before the first terminal.
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "done1", Title: "Done Early", Status: StatusCompleted},
			{ID: "1", Title: "First", Status: StatusBacklog},
			{ID: "2", Title: "Second", Status: StatusBacklog},
			{ID: "done2", Title: "Done Mid", Status: StatusCompleted},
			{ID: "3", Title: "Third", Status: StatusBacklog},
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
		t.Errorf("expected '1' last active, got %q", active[2].ID)
	}
}

func TestActiveTodosLegacy(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "1", Status: StatusBacklog},
			{ID: "2", Status: StatusCompleted},
			{ID: "3", Status: StatusNew},
		},
	}
	active := cc.ActiveTodos()
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
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

func TestAutoDescribePath(t *testing.T) {
	t.Run("go.mod returns Go project", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/user/repo\n"), 0o644)
		desc := AutoDescribePath(dir)
		if desc == "" {
			t.Fatal("expected non-empty description")
		}
		if !strContains(desc, "Go project") {
			t.Errorf("expected description containing 'Go project', got %q", desc)
		}
	})

	t.Run("package.json returns Node.js", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Node.js/JavaScript project" {
			t.Errorf("expected 'Node.js/JavaScript project', got %q", desc)
		}
	})

	t.Run("Cargo.toml returns Rust", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Rust project" {
			t.Errorf("expected 'Rust project', got %q", desc)
		}
	})

	t.Run("pyproject.toml returns Python", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Python project" {
			t.Errorf("expected 'Python project', got %q", desc)
		}
	})

	t.Run("Gemfile returns Ruby", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "Gemfile"), []byte(""), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Ruby project" {
			t.Errorf("expected 'Ruby project', got %q", desc)
		}
	})

	t.Run("pom.xml returns Java", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(""), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Java project" {
			t.Errorf("expected 'Java project', got %q", desc)
		}
	})

	t.Run("Package.swift returns Swift", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "Package.swift"), []byte(""), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "Swift project" {
			t.Errorf("expected 'Swift project', got %q", desc)
		}
	})

	t.Run("empty dir returns empty string", func(t *testing.T) {
		dir := t.TempDir()
		desc := AutoDescribePath(dir)
		if desc != "" {
			t.Errorf("expected empty string, got %q", desc)
		}
	})

	t.Run("random file returns empty string", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644)
		desc := AutoDescribePath(dir)
		if desc != "" {
			t.Errorf("expected empty string, got %q", desc)
		}
	})
}

// strContains is a test helper -- avoids importing strings just for Contains.
func strContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSwapTodoOrder(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	DBInsertTodo(database, Todo{ID: "a", Title: "First", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})
	DBInsertTodo(database, Todo{ID: "b", Title: "Second", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})
	DBInsertTodo(database, Todo{ID: "c", Title: "Third", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()})

	// Verify initial order: a, b, c
	cc, _ := LoadCommandCenterFromDB(database)
	if cc.Todos[0].ID != "a" || cc.Todos[1].ID != "b" || cc.Todos[2].ID != "c" {
		t.Fatalf("unexpected initial order: %s, %s, %s", cc.Todos[0].ID, cc.Todos[1].ID, cc.Todos[2].ID)
	}

	// Swap first two: a <-> b -> order becomes b, a, c
	if err := DBSwapTodoOrder(database, "a", "b"); err != nil {
		t.Fatalf("swap: %v", err)
	}

	cc, _ = LoadCommandCenterFromDB(database)
	if cc.Todos[0].ID != "b" || cc.Todos[1].ID != "a" || cc.Todos[2].ID != "c" {
		t.Fatalf("expected b, a, c after swap, got %s, %s, %s", cc.Todos[0].ID, cc.Todos[1].ID, cc.Todos[2].ID)
	}

	// Swap with itself -> no change, no error
	if err := DBSwapTodoOrder(database, "c", "c"); err != nil {
		t.Fatalf("self-swap should not error: %v", err)
	}

	cc, _ = LoadCommandCenterFromDB(database)
	if cc.Todos[2].ID != "c" {
		t.Fatalf("expected c still last after self-swap, got %s", cc.Todos[2].ID)
	}
}

func TestSwapPathOrder(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	DBAddPath(database, "/path/one")
	DBAddPath(database, "/path/two")
	DBAddPath(database, "/path/three")

	// Verify initial order
	paths, _ := DBLoadPaths(database)
	if paths[0] != "/path/one" || paths[1] != "/path/two" || paths[2] != "/path/three" {
		t.Fatalf("unexpected initial order: %v", paths)
	}

	// Swap last two: two <-> three
	if err := DBSwapPathOrder(database, "/path/two", "/path/three"); err != nil {
		t.Fatalf("swap: %v", err)
	}

	paths, _ = DBLoadPaths(database)
	if paths[0] != "/path/one" || paths[1] != "/path/three" || paths[2] != "/path/two" {
		t.Fatalf("expected one, three, two after swap, got %v", paths)
	}
}

func TestPullRequestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	now := time.Now().Truncate(time.Second)
	prs := []PullRequest{
		{
			ID:                    "owner/repo#42",
			Repo:                  "owner/repo",
			Number:                42,
			Title:                 "Add feature X",
			URL:                   "https://github.com/owner/repo/pull/42",
			Author:                "alice",
			Draft:                 false,
			CreatedAt:             now.Add(-48 * time.Hour),
			UpdatedAt:             now.Add(-1 * time.Hour),
			ReviewDecision:        "APPROVED",
			MyRole:                "author",
			ReviewerLogins:        []string{"bob", "carol"},
			PendingReviewerLogins: []string{"carol"},
			CommentCount:          5,
			UnresolvedThreadCount: 1,
			LastActivityAt:        now.Add(-30 * time.Minute),
			CIStatus:              "success",
			Category:              "waiting",
			FetchedAt:             now,
		},
		{
			ID:                    "owner/repo#43",
			Repo:                  "owner/repo",
			Number:                43,
			Title:                 "Fix bug Y",
			URL:                   "https://github.com/owner/repo/pull/43",
			Author:                "bob",
			Draft:                 true,
			CreatedAt:             now.Add(-24 * time.Hour),
			UpdatedAt:             now,
			ReviewDecision:        "",
			MyRole:                "reviewer",
			ReviewerLogins:        nil,
			PendingReviewerLogins: nil,
			CommentCount:          0,
			UnresolvedThreadCount: 0,
			LastActivityAt:        now,
			CIStatus:              "pending",
			Category:              "review",
			FetchedAt:             now,
		},
	}

	// Save via DBSaveRefreshResult
	cc := &CommandCenter{
		GeneratedAt:  now,
		PullRequests: prs,
	}
	if err := DBSaveRefreshResult(database, cc); err != nil {
		t.Fatalf("DBSaveRefreshResult: %v", err)
	}

	// Load back
	loaded, err := LoadCommandCenterFromDB(database)
	if err != nil {
		t.Fatalf("LoadCommandCenterFromDB: %v", err)
	}

	if len(loaded.PullRequests) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(loaded.PullRequests))
	}

	// PRs are ordered by last_activity_at DESC, so PR#43 (now) comes first
	pr43 := loaded.PullRequests[0]
	pr42 := loaded.PullRequests[1]

	if pr42.ID != "owner/repo#42" {
		t.Errorf("expected PR ID owner/repo#42, got %s", pr42.ID)
	}
	if pr42.Number != 42 {
		t.Errorf("expected number 42, got %d", pr42.Number)
	}
	if pr42.Author != "alice" {
		t.Errorf("expected author alice, got %s", pr42.Author)
	}
	if pr42.Draft {
		t.Error("expected draft=false for PR#42")
	}
	if pr42.ReviewDecision != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", pr42.ReviewDecision)
	}
	if pr42.MyRole != "author" {
		t.Errorf("expected author role, got %s", pr42.MyRole)
	}
	if pr42.CIStatus != "success" {
		t.Errorf("expected success CI, got %s", pr42.CIStatus)
	}
	if pr42.Category != "waiting" {
		t.Errorf("expected waiting category, got %s", pr42.Category)
	}

	// Verify JSON slice fields round-trip
	if len(pr42.ReviewerLogins) != 2 || pr42.ReviewerLogins[0] != "bob" || pr42.ReviewerLogins[1] != "carol" {
		t.Errorf("reviewer_logins mismatch: %v", pr42.ReviewerLogins)
	}
	if len(pr42.PendingReviewerLogins) != 1 || pr42.PendingReviewerLogins[0] != "carol" {
		t.Errorf("pending_reviewer_logins mismatch: %v", pr42.PendingReviewerLogins)
	}

	// Nil slices are saved as "[]" JSON, so they round-trip as empty (not nil)
	if len(pr43.ReviewerLogins) != 0 {
		t.Errorf("expected empty reviewer_logins for PR#43, got %v", pr43.ReviewerLogins)
	}
	if len(pr43.PendingReviewerLogins) != 0 {
		t.Errorf("expected empty pending_reviewer_logins for PR#43, got %v", pr43.PendingReviewerLogins)
	}

	if !pr43.Draft {
		t.Error("expected draft=true for PR#43")
	}
	if pr43.CommentCount != 0 {
		t.Errorf("expected 0 comments, got %d", pr43.CommentCount)
	}

	// Verify overwrite behavior — save empty list
	cc.PullRequests = nil
	if err := DBSaveRefreshResult(database, cc); err != nil {
		t.Fatalf("second save: %v", err)
	}
	loaded, _ = LoadCommandCenterFromDB(database)
	if len(loaded.PullRequests) != 0 {
		t.Fatalf("expected 0 PRs after overwrite, got %d", len(loaded.PullRequests))
	}
}

func TestPullRequestJSONSliceMarshaling(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	now := time.Now().Truncate(time.Second)

	// Test with empty slices vs nil slices
	prs := []PullRequest{
		{
			ID:                    "test/repo#1",
			Repo:                  "test/repo",
			Number:                1,
			Title:                 "Empty slices",
			URL:                   "https://github.com/test/repo/pull/1",
			Author:                "dev",
			CreatedAt:             now,
			UpdatedAt:             now,
			ReviewerLogins:        []string{},
			PendingReviewerLogins: []string{},
			LastActivityAt:        now,
			FetchedAt:             now,
		},
	}

	cc := &CommandCenter{GeneratedAt: now, PullRequests: prs}
	if err := DBSaveRefreshResult(database, cc); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadCommandCenterFromDB(database)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.PullRequests) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(loaded.PullRequests))
	}
	pr := loaded.PullRequests[0]

	// Empty slices marshal to "[]" and unmarshal to empty slice
	if pr.ReviewerLogins == nil || len(pr.ReviewerLogins) != 0 {
		t.Errorf("expected empty slice for reviewer_logins, got %v", pr.ReviewerLogins)
	}
	if pr.PendingReviewerLogins == nil || len(pr.PendingReviewerLogins) != 0 {
		t.Errorf("expected empty slice for pending_reviewer_logins, got %v", pr.PendingReviewerLogins)
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

func TestTodoMergesTableExists(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(`INSERT INTO cc_todo_merges (synthesis_id, original_id, vetoed, created_at)
		VALUES ('s1', 'o1', 0, '2026-03-19T00:00:00Z')`)
	if err != nil {
		t.Fatalf("cc_todo_merges table should exist: %v", err)
	}

	// Verify primary key constraint
	_, err = database.Exec(`INSERT INTO cc_todo_merges (synthesis_id, original_id, vetoed, created_at)
		VALUES ('s1', 'o1', 0, '2026-03-19T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected unique constraint violation on duplicate (synthesis_id, original_id)")
	}
}

func TestMergeCRUD(t *testing.T) {
	database, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert merges
	if err := DBInsertMerge(database, "synth-1", "orig-a", "same task"); err != nil {
		t.Fatalf("insert merge: %v", err)
	}
	if err := DBInsertMerge(database, "synth-1", "orig-b", "same task"); err != nil {
		t.Fatalf("insert merge: %v", err)
	}

	// Load merges
	merges, err := DBLoadMerges(database)
	if err != nil {
		t.Fatalf("load merges: %v", err)
	}
	if len(merges) != 2 {
		t.Fatalf("expected 2 merges, got %d", len(merges))
	}

	// Get originals for synthesis
	origIDs := DBGetOriginalIDs(merges, "synth-1")
	if len(origIDs) != 2 {
		t.Fatalf("expected 2 originals, got %d", len(origIDs))
	}

	// Veto one
	if err := DBSetMergeVetoed(database, "synth-1", "orig-a", true); err != nil {
		t.Fatalf("veto: %v", err)
	}
	merges, _ = DBLoadMerges(database)
	origIDs = DBGetOriginalIDs(merges, "synth-1")
	if len(origIDs) != 1 {
		t.Fatalf("expected 1 non-vetoed original, got %d", len(origIDs))
	}

	// Delete synthesis merges
	if err := DBDeleteSynthesisMerges(database, "synth-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	merges, _ = DBLoadMerges(database)
	if len(merges) != 0 {
		t.Fatalf("expected 0 merges after delete, got %d", len(merges))
	}
}

func TestWerePreviouslyMergedAndVetoed(t *testing.T) {
	merges := []TodoMerge{
		{SynthesisID: "s1", OriginalID: "a", Vetoed: true},
		{SynthesisID: "s1", OriginalID: "b", Vetoed: false},
	}
	// a and b were in same synthesis and a was vetoed — should be vetoed
	if !WerePreviouslyMergedAndVetoed(merges, "a", "b") {
		t.Error("expected vetoed for pair that was split")
	}
	// c and d are unrelated
	if WerePreviouslyMergedAndVetoed(merges, "c", "d") {
		t.Error("expected not vetoed for unrelated IDs")
	}
	// a and c were never in the same synthesis
	if WerePreviouslyMergedAndVetoed(merges, "a", "c") {
		t.Error("a's veto from s1 should not block merging a with c")
	}
}

func TestVisibleTodosHidesMergedOriginals(t *testing.T) {
	cc := &CommandCenter{
		Todos: []Todo{
			{ID: "a", Title: "Do X", Status: StatusBacklog},
			{ID: "b", Title: "Do X tomorrow", Status: StatusBacklog},
			{ID: "synth-1", Title: "Do X by tomorrow", Status: StatusBacklog, Source: "merge"},
			{ID: "c", Title: "Unrelated", Status: StatusBacklog},
		},
		Merges: []TodoMerge{
			{SynthesisID: "synth-1", OriginalID: "a", Vetoed: false},
			{SynthesisID: "synth-1", OriginalID: "b", Vetoed: false},
		},
	}

	visible := cc.VisibleTodos()
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible todos (synth-1 + c), got %d", len(visible))
	}

	// Veto one — now it should reappear
	cc.Merges[0].Vetoed = true
	visible = cc.VisibleTodos()
	// a is vetoed (visible), b still merged (hidden), synth-1 visible, c visible
	if len(visible) != 3 {
		t.Fatalf("expected 3 visible todos after veto, got %d", len(visible))
	}
}

func TestFullMergeCycle(t *testing.T) {
	database, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Create two originals
	a := Todo{ID: "a", DisplayID: 1, Title: "Do X", Status: StatusBacklog, Source: "manual", CreatedAt: time.Now()}
	b := Todo{ID: "b", DisplayID: 2, Title: "Do X tomorrow", Status: StatusBacklog, Source: "manual", Due: "2026-03-20", CreatedAt: time.Now()}
	if err := DBInsertTodo(database, a); err != nil {
		t.Fatalf("insert a: %v", err)
	}
	if err := DBInsertTodo(database, b); err != nil {
		t.Fatalf("insert b: %v", err)
	}

	// Create synthesis
	s := Todo{ID: "s1", DisplayID: 3, Title: "Do X by tomorrow", Status: StatusBacklog, Source: "merge", Due: "2026-03-20", CreatedAt: time.Now()}
	if err := DBInsertTodo(database, s); err != nil {
		t.Fatalf("insert synth: %v", err)
	}
	if err := DBInsertMerge(database, "s1", "a", "same task"); err != nil {
		t.Fatalf("insert merge a: %v", err)
	}
	if err := DBInsertMerge(database, "s1", "b", "same task"); err != nil {
		t.Fatalf("insert merge b: %v", err)
	}

	// Load and verify visibility
	cc, err := LoadCommandCenterFromDB(database)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	visible := cc.VisibleTodos()
	visibleIDs := make(map[string]bool)
	for _, v := range visible {
		visibleIDs[v.ID] = true
	}
	if visibleIDs["a"] || visibleIDs["b"] {
		t.Error("originals should be hidden")
	}
	if !visibleIDs["s1"] {
		t.Error("synthesis should be visible")
	}

	// Unmerge b (veto)
	if err := DBSetMergeVetoed(database, "s1", "b", true); err != nil {
		t.Fatalf("veto: %v", err)
	}
	cc, _ = LoadCommandCenterFromDB(database)
	visible = cc.VisibleTodos()
	visibleIDs = make(map[string]bool)
	for _, v := range visible {
		visibleIDs[v.ID] = true
	}
	if !visibleIDs["b"] {
		t.Error("b should be visible after veto")
	}
	if visibleIDs["a"] {
		t.Error("a should still be hidden")
	}
	if !visibleIDs["s1"] {
		t.Error("synthesis should still be visible")
	}

	// Verify WerePreviouslyMergedAndVetoed
	if !WerePreviouslyMergedAndVetoed(cc.Merges, "a", "b") {
		t.Error("a and b should be vetoed (b was vetoed from their shared synthesis)")
	}

	// Cleanup: delete synthesis and merges
	if err := DBDeleteSynthesisMerges(database, "s1"); err != nil {
		t.Fatalf("delete merges: %v", err)
	}
	if err := DBDeleteTodo(database, "s1"); err != nil {
		t.Fatalf("delete synth: %v", err)
	}
	cc, _ = LoadCommandCenterFromDB(database)
	visible = cc.VisibleTodos()
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible todos after cleanup (a + b), got %d", len(visible))
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
