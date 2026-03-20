# Todo Duplicate Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect and merge duplicate todos across manual intake and external sources using LLM-based semantic matching with synthesis-based, reversible merges.

**Architecture:** Originals are immutable. When a duplicate is detected, a synthesis todo is created combining all originals (newest wins on overlap). The `cc_todo_merges` table tracks which originals feed each synthesis. Unmerge deletes the synthesis and restores originals. Detection piggybacks on existing enrichment/routing LLM calls; synthesis uses one additional Haiku call.

**Tech Stack:** Go, SQLite, bubbletea TUI, Claude LLM (Haiku for synthesis, existing models for detection)

**Spec:** `docs/superpowers/specs/2026-03-19-todo-duplicate-detection-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/db/schema.go` | Modify | Add `cc_todo_merges` table creation |
| `internal/db/types.go` | Modify | Add `TodoMerge` type, `VisibleTodos()` method, update `ActiveTodos()` |
| `internal/db/write.go` | Modify | Add merge CRUD functions |
| `internal/db/read.go` | Modify | Add `dbLoadTodoMerges()`, load merges into `CommandCenter` |
| `internal/db/db_test.go` | Modify | Tests for merge CRUD and visibility filtering |
| `internal/refresh/todo_agent.go` | Modify | Add active todo list to routing prompt, parse `merge_into` |
| `internal/refresh/merge.go` | Modify | Handle `merge_into` in `mergeTodos` |
| `internal/refresh/todo_synthesis.go` | Create | Synthesis LLM call — takes originals, produces combined todo |
| `internal/refresh/todo_synthesis_test.go` | Create | Tests for synthesis prompt building and result parsing |
| `internal/refresh/refresh.go` | Modify | Add dedup pass after routing, call synthesis |
| `internal/refresh/merge_test.go` | Modify | Tests for merge-aware todo merging |
| `internal/builtin/commandcenter/claude_exec.go` | Modify | Expand `buildEnrichPrompt` to include active todos and `merge_into` |
| `internal/builtin/commandcenter/cc_messages.go` | Modify | Handle `merge_into` in enrich handler, trigger synthesis |
| `internal/builtin/commandcenter/cc_keys_detail.go` | Modify | Add `U` keybinding for unmerge |
| `internal/builtin/commandcenter/cc_view.go` | Modify | Render "Sources" section in detail view |
| `internal/builtin/commandcenter/commandcenter.go` | Modify | Search filter matches display_id, flash message includes display_id |
| `internal/builtin/commandcenter/commandcenter_test.go` | Modify | Tests for search-by-id and flash messages |

---

## Task 1: Schema — `cc_todo_merges` table

**Files:**
- Modify: `internal/db/schema.go`
- Modify: `internal/db/types.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write failing test for merge table existence**

In `internal/db/db_test.go`, add:

```go
func TestTodoMergesTableExists(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	_, err := database.Exec(`INSERT INTO cc_todo_merges (synthesis_id, original_id, vetoed, created_at)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestTodoMergesTableExists -v`
Expected: FAIL — table does not exist

- [ ] **Step 3: Add TodoMerge type to types.go**

In `internal/db/types.go`, add after the `Todo` struct:

```go
// TodoMerge tracks which original todos have been merged into a synthesis todo.
type TodoMerge struct {
	SynthesisID string `json:"synthesis_id"`
	OriginalID  string `json:"original_id"`
	Vetoed      bool   `json:"vetoed"`
	CreatedAt   string `json:"created_at"`
}
```

Add `Merges []TodoMerge` field to `CommandCenter` struct.

- [ ] **Step 4: Add CREATE TABLE to schema.go**

In `internal/db/schema.go`, inside `EnsureSchema`, add after the `cc_todos` table creation:

```go
CREATE TABLE IF NOT EXISTS cc_todo_merges (
    synthesis_id TEXT NOT NULL,
    original_id TEXT NOT NULL,
    vetoed INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    PRIMARY KEY (synthesis_id, original_id)
);
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestTodoMergesTableExists -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/db/schema.go internal/db/types.go internal/db/db_test.go
git commit -m "Add cc_todo_merges table and TodoMerge type"
```

---

## Task 2: Merge CRUD — read/write/delete operations

**Files:**
- Modify: `internal/db/write.go`
- Modify: `internal/db/read.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write failing tests for merge CRUD**

In `internal/db/db_test.go`:

```go
func TestMergeCRUD(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	// Insert a merge
	err := DBInsertMerge(database, "synth-1", "orig-a", "same task")
	if err != nil {
		t.Fatalf("insert merge: %v", err)
	}
	err = DBInsertMerge(database, "synth-1", "orig-b", "same task")
	if err != nil {
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

	// Get originals for a synthesis
	origIDs := DBGetOriginalIDs(merges, "synth-1")
	if len(origIDs) != 2 {
		t.Fatalf("expected 2 originals, got %d", len(origIDs))
	}

	// Veto one
	err = DBSetMergeVetoed(database, "synth-1", "orig-a", true)
	if err != nil {
		t.Fatalf("veto: %v", err)
	}

	merges, _ = DBLoadMerges(database)
	origIDs = DBGetOriginalIDs(merges, "synth-1")
	if len(origIDs) != 1 {
		t.Fatalf("expected 1 non-vetoed original after veto, got %d", len(origIDs))
	}

	// Delete synthesis merges
	err = DBDeleteSynthesisMerges(database, "synth-1")
	if err != nil {
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
	// a and b were in the same synthesis and a was vetoed — should be vetoed
	if !WerePreviouslyMergedAndVetoed(merges, "a", "b") {
		t.Error("expected vetoed for pair that was split")
	}
	// c and d are unrelated — should not be vetoed
	if WerePreviouslyMergedAndVetoed(merges, "c", "d") {
		t.Error("expected not vetoed for unrelated IDs")
	}
	// a and c were never in the same synthesis — should not be vetoed
	// (a was vetoed from s1 but c was never in s1)
	if WerePreviouslyMergedAndVetoed(merges, "a", "c") {
		t.Error("expected not vetoed — a's veto from s1 should not block merging a with c")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run "TestMergeCRUD|TestIsVetoed" -v`
Expected: FAIL — functions don't exist

- [ ] **Step 3: Implement merge CRUD in write.go**

In `internal/db/write.go`:

```go
func DBInsertMerge(db *sql.DB, synthesisID, originalID, mergeNote string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(`INSERT OR REPLACE INTO cc_todo_merges (synthesis_id, original_id, vetoed, created_at)
		VALUES (?, ?, 0, ?)`, synthesisID, originalID, now)
	return err
}

func DBSetMergeVetoed(db *sql.DB, synthesisID, originalID string, vetoed bool) error {
	v := 0
	if vetoed {
		v = 1
	}
	_, err := db.Exec(`UPDATE cc_todo_merges SET vetoed = ? WHERE synthesis_id = ? AND original_id = ?`,
		v, synthesisID, originalID)
	return err
}

func DBDeleteSynthesisMerges(db *sql.DB, synthesisID string) error {
	_, err := db.Exec(`DELETE FROM cc_todo_merges WHERE synthesis_id = ?`, synthesisID)
	return err
}

func DBDeleteTodo(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM cc_todos WHERE id = ?`, id)
	return err
}
```

- [ ] **Step 4: Implement merge read + helpers in read.go and types.go**

In `internal/db/read.go`:

```go
func DBLoadMerges(database *sql.DB) ([]TodoMerge, error) {
	rows, err := database.Query(`SELECT synthesis_id, original_id, vetoed, created_at FROM cc_todo_merges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var merges []TodoMerge
	for rows.Next() {
		var m TodoMerge
		var vetoed int
		if err := rows.Scan(&m.SynthesisID, &m.OriginalID, &vetoed, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Vetoed = vetoed != 0
		merges = append(merges, m)
	}
	return merges, rows.Err()
}
```

In `internal/db/types.go`:

```go
// DBGetOriginalIDs returns the non-vetoed original IDs for a synthesis.
func DBGetOriginalIDs(merges []TodoMerge, synthesisID string) []string {
	var ids []string
	for _, m := range merges {
		if m.SynthesisID == synthesisID && !m.Vetoed {
			ids = append(ids, m.OriginalID)
		}
	}
	return ids
}

// WerePreviouslyMergedAndVetoed checks if two IDs were ever in the same
// synthesis and one of them was vetoed out. This prevents re-merging a
// specific pair while allowing each ID to merge with unrelated todos.
func WerePreviouslyMergedAndVetoed(merges []TodoMerge, idA, idB string) bool {
	// Group originals by synthesis_id
	synthGroups := make(map[string][]TodoMerge)
	for _, m := range merges {
		synthGroups[m.SynthesisID] = append(synthGroups[m.SynthesisID], m)
	}
	// Check if any synthesis contained both IDs and at least one was vetoed
	for _, group := range synthGroups {
		hasA, hasB, hasVeto := false, false, false
		for _, m := range group {
			if m.OriginalID == idA { hasA = true; if m.Vetoed { hasVeto = true } }
			if m.OriginalID == idB { hasB = true; if m.Vetoed { hasVeto = true } }
		}
		if hasA && hasB && hasVeto {
			return true
		}
	}
	return false
}
```

Also load merges in `LoadCommandCenterFromDB`:

```go
merges, err := DBLoadMerges(db)
if err != nil {
    return nil, fmt.Errorf("load merges: %w", err)
}
cc.Merges = merges
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/db/ -run "TestMergeCRUD|TestIsVetoed" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/db/write.go internal/db/read.go internal/db/types.go internal/db/db_test.go
git commit -m "Add merge CRUD operations and helpers"
```

---

## Task 3: Visibility filtering — hide merged originals

**Files:**
- Modify: `internal/db/types.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write failing test for VisibleTodos**

In `internal/db/db_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestVisibleTodosHidesMergedOriginals -v`
Expected: FAIL — `VisibleTodos` not defined

- [ ] **Step 3: Implement VisibleTodos**

In `internal/db/types.go`:

```go
// VisibleTodos returns todos not hidden by a non-vetoed merge.
func (cc *CommandCenter) VisibleTodos() []Todo {
	hidden := make(map[string]bool)
	for _, m := range cc.Merges {
		if !m.Vetoed {
			hidden[m.OriginalID] = true
		}
	}
	var out []Todo
	for _, t := range cc.Todos {
		if !hidden[t.ID] {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Update ActiveTodos to use VisibleTodos as base**

Change `ActiveTodos` to filter from visible todos only:

```go
func (cc *CommandCenter) ActiveTodos() []Todo {
	var out []Todo
	for _, t := range cc.VisibleTodos() {
		if !IsTerminalStatus(t.Status) {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 5: Run all db tests**

Run: `go test ./internal/db/ -v`
Expected: ALL PASS (existing tests should still work since no merges = no filtering)

- [ ] **Step 6: Commit**

```bash
git add internal/db/types.go internal/db/db_test.go
git commit -m "Add VisibleTodos filtering for merged originals"
```

---

## Task 4: Synthesis LLM — combine originals into one todo

**Files:**
- Create: `internal/refresh/todo_synthesis.go`
- Create: `internal/refresh/todo_synthesis_test.go`

- [ ] **Step 1: Write failing test for synthesis prompt building**

Create `internal/refresh/todo_synthesis_test.go`:

```go
package refresh

import (
	"strings"
	"testing"

	"github.com/anutron/claude-command-center/internal/db"
)

func TestBuildSynthesisPrompt(t *testing.T) {
	originals := []db.Todo{
		{ID: "a", DisplayID: 12, Title: "Do X", Source: "manual"},
		{ID: "b", DisplayID: 14, Title: "Do X for Carol by Monday", Source: "slack",
			Due: "2026-03-24", WhoWaiting: "Carol"},
	}

	prompt := buildSynthesisPrompt(originals)

	if !strings.Contains(prompt, "#12") {
		t.Error("prompt should reference display IDs")
	}
	if !strings.Contains(prompt, "Do X for Carol") {
		t.Error("prompt should contain original titles")
	}
	if !strings.Contains(prompt, "newest entry is the source of truth") {
		t.Error("prompt should instruct newest-wins on overlap")
	}
}

func TestParseSynthesisResult(t *testing.T) {
	raw := `{"title":"Do X for Carol","due":"2026-03-24","who_waiting":"Carol","detail":"Combined from manual + slack","context":"","effort":""}`
	result, err := parseSynthesisResult(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.Title != "Do X for Carol" {
		t.Errorf("expected title 'Do X for Carol', got %q", result.Title)
	}
	if result.Due != "2026-03-24" {
		t.Errorf("expected due '2026-03-24', got %q", result.Due)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/refresh/ -run "TestBuildSynthesisPrompt|TestParseSynthesisResult" -v`
Expected: FAIL — functions don't exist

- [ ] **Step 3: Implement synthesis module**

Create `internal/refresh/todo_synthesis.go`:

```go
package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// SynthesisResult is the LLM output for a combined todo.
type SynthesisResult struct {
	Title      string `json:"title"`
	Due        string `json:"due"`
	WhoWaiting string `json:"who_waiting"`
	Detail     string `json:"detail"`
	Context    string `json:"context"`
	Effort     string `json:"effort"`
}

// Synthesize calls the LLM to combine multiple originals into one todo.
// The newest original (last in slice) is source of truth on overlap.
func Synthesize(ctx context.Context, l llm.LLM, originals []db.Todo) (*SynthesisResult, error) {
	prompt := buildSynthesisPrompt(originals)
	text, err := l.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("synthesis LLM call: %w", err)
	}
	return parseSynthesisResult(CleanJSON(text))
}

func buildSynthesisPrompt(originals []db.Todo) string {
	var b strings.Builder
	b.WriteString(`Combine these related todos into one. The newest entry (last in the list) is the source of truth where information overlaps. Fill gaps from older entries.

Originals (oldest first):
`)
	for i, t := range originals {
		fmt.Fprintf(&b, "%d. [#%d] %q (source: %s", i+1, t.DisplayID, t.Title, t.Source)
		if t.Due != "" {
			fmt.Fprintf(&b, ", due: %s", t.Due)
		}
		if t.WhoWaiting != "" {
			fmt.Fprintf(&b, ", who_waiting: %s", t.WhoWaiting)
		}
		if t.Effort != "" {
			fmt.Fprintf(&b, ", effort: %s", t.Effort)
		}
		if t.Detail != "" {
			fmt.Fprintf(&b, ", detail: %s", t.Detail)
		}
		b.WriteString(")\n")
	}
	b.WriteString(`
Return a single combined todo as JSON with these fields:
- title: concise action item
- due: YYYY-MM-DD or empty string
- who_waiting: person name(s) or empty string
- detail: comprehensive background combining all sources
- context: short categorization
- effort: estimated effort or empty string

Output ONLY the JSON object, no markdown code fences, no explanation.`)
	return b.String()
}

func parseSynthesisResult(raw string) (*SynthesisResult, error) {
	var result SynthesisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parsing synthesis result: %w (raw: %s)", err, raw[:min(200, len(raw))])
	}
	return &result, nil
}

// BuildSynthesisTodo creates a new Todo from a synthesis result.
// Status comes from mergeTarget (preserves triage decisions).
// Non-LLM fields come from the newest original.
func BuildSynthesisTodo(result *SynthesisResult, originals []db.Todo, mergeTarget *db.Todo) db.Todo {
	newest := originals[len(originals)-1]

	// Status: inherit from the merge target (the existing visible todo),
	// not from the newest original (which is always StatusBacklog from AddTodo).
	status := mergeTarget.Status

	return db.Todo{
		ID:             db.GenID(),
		DisplayID:      0, // DB auto-assigns via MAX(display_id)+1 on insert
		Title:          result.Title,
		Status:         status,
		Source:         "merge",
		Detail:         result.Detail,
		Context:        result.Context,
		WhoWaiting:     result.WhoWaiting,
		Due:            result.Due,
		Effort:         result.Effort,
		ProjectDir:     newest.ProjectDir,
		ProposedPrompt: newest.ProposedPrompt,
		SourceContext:   newest.SourceContext,
		SourceContextAt: newest.SourceContextAt,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refresh/ -run "TestBuildSynthesisPrompt|TestParseSynthesisResult" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/refresh/todo_synthesis.go internal/refresh/todo_synthesis_test.go
git commit -m "Add todo synthesis LLM module"
```

---

## Task 5: Enrichment prompt — add active todos and merge_into field

**Files:**
- Modify: `internal/builtin/commandcenter/claude_exec.go`
- Test: `internal/builtin/commandcenter/commandcenter_test.go`

- [ ] **Step 1: Write failing test for enrichment prompt containing active todos**

In `internal/builtin/commandcenter/commandcenter_test.go`:

```go
func TestBuildEnrichPromptIncludesActiveTodos(t *testing.T) {
	todos := []db.Todo{
		{ID: "a", DisplayID: 12, Title: "Send report to Bob"},
		{ID: "b", DisplayID: 13, Title: "Review PR"},
	}
	prompt := buildEnrichPrompt("do the report thing", todos)

	if !strings.Contains(prompt, "#12") {
		t.Error("prompt should contain display ID #12")
	}
	if !strings.Contains(prompt, "Send report to Bob") {
		t.Error("prompt should contain existing todo title")
	}
	if !strings.Contains(prompt, "merge_into") {
		t.Error("prompt should ask for merge_into field")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/builtin/commandcenter/ -run TestBuildEnrichPromptIncludesActiveTodos -v`
Expected: FAIL — `buildEnrichPrompt` signature doesn't accept todos

- [ ] **Step 3: Update buildEnrichPrompt signature and prompt text**

In `internal/builtin/commandcenter/claude_exec.go`, change `buildEnrichPrompt` to accept active todos:

```go
func buildEnrichPrompt(rawText string, activeTodos []db.Todo) string {
```

Add to the prompt, before the `Text:` section:

```go
var todoSection string
if len(activeTodos) > 0 {
    var sb strings.Builder
    sb.WriteString("\n## Existing Todos\n")
    sb.WriteString("If the new item is semantically the same as an existing todo, return its ID in merge_into.\n\n")
    limit := len(activeTodos)
    if limit > 50 {
        limit = 50
    }
    for _, t := range activeTodos[:limit] {
        fmt.Fprintf(&sb, "- [#%d] (id: %s) %s", t.DisplayID, t.ID, t.Title)
        if t.Due != "" {
            fmt.Fprintf(&sb, " (due: %s)", t.Due)
        }
        sb.WriteString("\n")
    }
    todoSection = sb.String()
}
```

Add `merge_into` and `merge_note` to the JSON fields list in the prompt:

```
- merge_into: ID of an existing todo if this is a duplicate, otherwise empty string
- merge_note: brief explanation of why this matches the existing todo, otherwise empty string
```

Update the call site in `cc_keys.go:651` to pass active todos:

```go
prompt := buildEnrichPrompt(text, p.cc.ActiveTodos())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/builtin/commandcenter/ -run TestBuildEnrichPromptIncludesActiveTodos -v`
Expected: PASS

- [ ] **Step 5: Run all commandcenter tests to check for regressions**

Run: `go test ./internal/builtin/commandcenter/ -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/builtin/commandcenter/claude_exec.go internal/builtin/commandcenter/cc_keys.go internal/builtin/commandcenter/commandcenter_test.go
git commit -m "Expand enrichment prompt with active todos and merge_into field"
```

---

## Task 6: Enrich handler — handle merge_into, trigger synthesis

**Files:**
- Modify: `internal/builtin/commandcenter/cc_messages.go`
- Modify: `internal/builtin/commandcenter/commandcenter.go` (add db field for merge writes)

- [ ] **Step 1: Update enriched struct to include merge fields**

In `cc_messages.go`, `handleClaudeEnrichFinished`, expand the enriched struct:

```go
var enriched struct {
    Title          string `json:"title"`
    Due            string `json:"due"`
    WhoWaiting     string `json:"who_waiting"`
    Effort         string `json:"effort"`
    Context        string `json:"context"`
    Detail         string `json:"detail"`
    ProjectDir     string `json:"project_dir"`
    ProposedPrompt string `json:"proposed_prompt"`
    MergeInto      string `json:"merge_into"`
    MergeNote      string `json:"merge_note"`
}
```

- [ ] **Step 2: Add merge handling logic after todo creation**

After the existing `todo := p.cc.AddTodo(enriched.Title)` block, add merge logic:

```go
if enriched.MergeInto != "" {
    // Validate merge target exists and isn't vetoed
    target := p.cc.FindTodo(enriched.MergeInto)
    if target != nil {
        // Check veto — but for manual intake, user intent overrides vetoes.
        // For manual intake the user explicitly typed something the LLM matched,
        // so clear any existing veto between these originals.
        // (Refresh path should respect vetoes since there's no explicit user intent.)
        //
        // Gather the originals first to check vetoes against ALL of them.
        // Gather originals: if target is a synthesis, get its originals; otherwise just the target
        var originals []db.Todo
        if target.Source == "merge" {
            origIDs := db.DBGetOriginalIDs(p.cc.Merges, target.ID)
            for _, oid := range origIDs {
                if orig := p.cc.FindTodo(oid); orig != nil {
                    originals = append(originals, *orig)
                }
            }
        } else {
            originals = []db.Todo{*target}
        }
        originals = append(originals, todoCopy) // newest last

        // Trigger async synthesis
        return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(
            p.dbWriteCmd(func(database *sql.DB) error {
                return db.DBInsertTodo(database, todoCopy)
            }),
            claudeSynthesizeCmd(p.llm, originals, target),
        )}
    }
}
// Fall through to normal (non-merge) path
```

- [ ] **Step 3: Add claudeSynthesizeCmd and handler**

Add a new command and message type for async synthesis:

```go
type claudeSynthesizeFinishedMsg struct {
    synthesis   db.Todo
    originals   []db.Todo
    oldSynthID  string // empty if target was a plain todo
    err         error
}

func claudeSynthesizeCmd(l llm.LLM, originals []db.Todo, target *db.Todo) tea.Cmd {
    return func() tea.Msg {
        result, err := refresh.Synthesize(context.Background(), l, originals)
        if err != nil {
            return claudeSynthesizeFinishedMsg{err: err}
        }
        synth := refresh.BuildSynthesisTodo(result, originals, target)
        oldSynthID := ""
        if target.Source == "merge" {
            oldSynthID = target.ID
        }
        return claudeSynthesizeFinishedMsg{synthesis: synth, originals: originals, oldSynthID: oldSynthID}
    }
}
```

Add handler for the synthesis result — creates the synthesis todo, writes merge rows, supersedes old synthesis:

```go
func (p *Plugin) handleSynthesizeFinished(msg claudeSynthesizeFinishedMsg) (bool, plugin.Action) {
    if msg.err != nil {
        p.flashMessage = "Merge synthesis failed: " + msg.err.Error()
        p.flashMessageAt = time.Now()
        return true, plugin.NoopAction()
    }
    synth := msg.synthesis
    synthCopy := synth
    p.cc.Todos = append(p.cc.Todos, synth)
    p.flashMessage = fmt.Sprintf("Merged with #%d: %s", synth.DisplayID, synth.Title)
    p.flashMessageAt = time.Now()

    return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: p.dbWriteCmd(func(database *sql.DB) error {
        // Delete old synthesis if superseding
        if msg.oldSynthID != "" {
            _ = db.DBDeleteSynthesisMerges(database, msg.oldSynthID)
            _ = db.DBDeleteTodo(database, msg.oldSynthID)
        }
        // Insert new synthesis todo
        if err := db.DBInsertTodo(database, synthCopy); err != nil {
            return err
        }
        // Insert merge rows for all originals
        for _, orig := range msg.originals {
            if err := db.DBInsertMerge(database, synthCopy.ID, orig.ID, ""); err != nil {
                return err
            }
        }
        return nil
    })}
}
```

- [ ] **Step 4: Add FindTodo helper to types.go**

In `internal/db/types.go`:

```go
func (cc *CommandCenter) FindTodo(id string) *Todo {
    for i := range cc.Todos {
        if cc.Todos[i].ID == id {
            return &cc.Todos[i]
        }
    }
    return nil
}
```

- [ ] **Step 5: Wire the handler in the message switch**

In `cc_messages.go`, add case for `claudeSynthesizeFinishedMsg` in the Update method.

- [ ] **Step 6: Update flash message for non-merge case**

Change the existing flash message from `"Added todo"` to include display_id:

```go
p.flashMessage = fmt.Sprintf("Added todo #%d", todoCopy.DisplayID)
```

- [ ] **Step 7: Run all tests**

Run: `go test ./internal/builtin/commandcenter/ -v && go test ./internal/db/ -v`
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add internal/builtin/commandcenter/cc_messages.go internal/builtin/commandcenter/commandcenter.go internal/db/types.go
git commit -m "Handle merge_into in enrichment — trigger synthesis on duplicate detection"
```

---

## Task 7: Refresh path — add dedup to routing prompt

**Files:**
- Modify: `internal/refresh/todo_agent.go`
- Modify: `internal/refresh/refresh.go`
- Modify: `internal/refresh/merge.go`
- Test: `internal/refresh/merge_test.go`

- [ ] **Step 1: Write failing test for routing prompt including active todos**

In a new test or existing `internal/refresh/todo_agent_test.go` (create if needed):

```go
func TestBuildRoutingPromptIncludesActiveTodos(t *testing.T) {
    todo := db.Todo{Title: "Do X", Source: "slack", SourceRef: "slack-123"}
    paths := PathContext{}
    activeTodos := []db.Todo{
        {ID: "a", DisplayID: 12, Title: "Do X already exists"},
    }
    prompt := buildRoutingPrompt(todo, paths, activeTodos)

    if !strings.Contains(prompt, "#12") {
        t.Error("prompt should contain existing todo display ID")
    }
    if !strings.Contains(prompt, "merge_into") {
        t.Error("prompt should ask for merge_into field")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/refresh/ -run TestBuildRoutingPromptIncludesActiveTodos -v`
Expected: FAIL — signature mismatch

- [ ] **Step 3: Update buildRoutingPrompt and TodoPromptResult**

In `todo_agent.go`:

Add `activeTodos []db.Todo` parameter to `buildRoutingPrompt`. Add an "Existing Todos" section to the prompt (same format as enrichment). Add `merge_into` and `merge_note` to the JSON instructions.

Update `TodoPromptResult`:

```go
type TodoPromptResult struct {
    ProjectDir     string `json:"project_dir"`
    ProposedPrompt string `json:"proposed_prompt"`
    Reasoning      string `json:"reasoning"`
    MergeInto      string `json:"merge_into"`
    MergeNote      string `json:"merge_note"`
}
```

Update `GenerateTodoPrompt` to accept and pass through active todos.

- [ ] **Step 4: Add dedup pass in refresh.go**

In `refresh.go`, add a new function called after `generateProposedPrompts`. This function needs the DB for loading existing merges and writing new ones.

```go
// dedupTodos checks routing results for merge_into suggestions, validates them,
// and creates synthesis todos for confirmed matches. Returns the updated todo list.
func dedupTodos(ctx context.Context, l llm.LLM, database *sql.DB, todos []db.Todo, merges []db.TodoMerge) []db.Todo {
	// Build lookup of visible todos by ID
	todoByID := make(map[string]*db.Todo)
	for i := range todos {
		todoByID[todos[i].ID] = &todos[i]
	}

	// Group merge suggestions: target_id -> []new_originals
	type mergeGroup struct {
		target    *db.Todo
		newTodos  []db.Todo
	}
	groups := make(map[string]*mergeGroup)

	for i, t := range todos {
		if t.MergeInto == "" || t.MergeInto == t.ID {
			continue
		}
		target, ok := todoByID[t.MergeInto]
		if !ok {
			continue // invalid target
		}
		// Veto check — refresh respects vetoes (unlike manual intake)
		if db.WerePreviouslyMergedAndVetoed(merges, t.ID, target.ID) {
			todos[i].MergeInto = "" // clear invalid suggestion
			continue
		}
		if _, exists := groups[target.ID]; !exists {
			groups[target.ID] = &mergeGroup{target: target}
		}
		groups[target.ID].newTodos = append(groups[target.ID].newTodos, t)
	}

	for _, g := range groups {
		// Gather all originals: if target is a synthesis, get its originals
		var originals []db.Todo
		if g.target.Source == "merge" {
			origIDs := db.DBGetOriginalIDs(merges, g.target.ID)
			for _, oid := range origIDs {
				if orig := todoByID[oid]; orig != nil {
					originals = append(originals, *orig)
				}
			}
		} else {
			originals = []db.Todo{*g.target}
		}
		originals = append(originals, g.newTodos...) // newest last

		// Synthesize
		result, err := Synthesize(ctx, l, originals)
		if err != nil {
			log.Printf("dedup synthesis for %q: %v", g.target.ID, err)
			continue
		}
		synth := BuildSynthesisTodo(result, originals, g.target)
		synth.CreatedAt = time.Now()

		// Write to DB: insert synthesis, merge rows, delete old synthesis
		if err := db.DBInsertTodo(database, synth); err != nil {
			log.Printf("dedup insert synthesis: %v", err)
			continue
		}
		for _, orig := range originals {
			_ = db.DBInsertMerge(database, synth.ID, orig.ID, "")
		}
		if g.target.Source == "merge" {
			_ = db.DBDeleteSynthesisMerges(database, g.target.ID)
			_ = db.DBDeleteTodo(database, g.target.ID)
		}

		// Update in-memory list: add synthesis, mark originals for hiding
		todos = append(todos, synth)
	}
	return todos
}
```

Call this in `refresh.go` after `generateProposedPrompts`, passing `opts.LLM` (Haiku) for synthesis:

```go
if opts.LLM != nil && opts.DB != nil && len(merged.Todos) > 0 {
    existingMerges, _ := db.DBLoadMerges(opts.DB)
    merged.Todos = dedupTodos(ctx, opts.LLM, opts.DB, merged.Todos, existingMerges)
}
```

- [ ] **Step 5: Pass active todos to GenerateTodoPrompt**

Update the call in `generateProposedPrompts` (line ~104 of `llm.go`) to load and pass active todos:

```go
// Before the loop, build active todo list for dedup
activeTodos := make([]db.Todo, 0)
for _, t := range todos {
    if !db.IsTerminalStatus(t.Status) {
        activeTodos = append(activeTodos, t)
    }
}

for _, i := range eligible {
    result, err := GenerateTodoPrompt(ctx, l, todos[i], pathCtx, activeTodos)
```

Update `GenerateTodoPrompt` signature to accept and pass `activeTodos`:

```go
func GenerateTodoPrompt(ctx context.Context, l llm.LLM, todo db.Todo, paths PathContext, activeTodos []db.Todo) (*TodoPromptResult, error) {
    prompt := buildRoutingPrompt(todo, paths, activeTodos)
```

Store the merge_into result back on the todo so `dedupTodos` can read it:

```go
if result.MergeInto != "" {
    todos[i].MergeInto = result.MergeInto
}
```

Note: This requires adding a `MergeInto` field to `db.Todo` (transient, not persisted to DB — just used to pass the routing result through to dedup). Add `MergeInto string `json:"-"`` to the Todo struct in `types.go`.

- [ ] **Step 6: Update merge.go to preserve merge source info**

In `mergeTodos`, when a fresh todo has a `Source == "merge"`, preserve it through the merge (don't overwrite with source from the fresh data).

- [ ] **Step 6: Run all refresh tests**

Run: `go test ./internal/refresh/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/refresh/todo_agent.go internal/refresh/refresh.go internal/refresh/merge.go internal/refresh/merge_test.go
git commit -m "Add duplicate detection to refresh routing path"
```

---

## Task 8: Detail view — show sources and unmerge keybinding

**Files:**
- Modify: `internal/builtin/commandcenter/cc_view.go`
- Modify: `internal/builtin/commandcenter/cc_keys_detail.go`

- [ ] **Step 1: Add "Sources" section to detail view**

In `cc_view.go`, in the todo detail rendering function, add after existing sections:

```go
// Only show for synthesis todos with merged children
if todo.Source == "merge" {
    origIDs := db.DBGetOriginalIDs(p.cc.Merges, todo.ID)
    if len(origIDs) > 0 {
        // Render "Sources" header and list of originals
        // Each line: "#12 — Do X (manual)" with cursor support for U keybinding
    }
}
```

- [ ] **Step 2: Add `U` keybinding for unmerge in detail view**

In `cc_keys_detail.go`, add case for `"U"`:

```go
case "U":
    if todo := p.detailTodo(); todo != nil && todo.Source == "merge" {
        origIDs := db.DBGetOriginalIDs(p.cc.Merges, todo.ID)
        if len(origIDs) > 0 {
            // Get the selected original from the sources list cursor
            selectedOrigID := origIDs[p.mergeSourceCursor]
            // Set vetoed, potentially re-synthesize or delete synthesis
            // ...
        }
    }
```

Implement the full unmerge logic:
1. Set `vetoed = 1` for the selected original
2. Count remaining non-vetoed originals
3. If 2+: re-synthesize from remaining (async LLM call)
4. If 1: delete synthesis, restore last original
5. If 0: delete synthesis

- [ ] **Step 3: Add `mergeSourceCursor` field to Plugin struct**

In `commandcenter.go`, add `mergeSourceCursor int` to track which source is selected in the detail view.

- [ ] **Step 4: Add keybinding hint conditionally**

In the detail view help section, only show `U — Unmerge` when viewing a synthesis todo with sources.

- [ ] **Step 5: Run all tests**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 6: Manual test**

Build and run: `make build && ccc`
- Create a todo, verify detail view does NOT show sources section
- (Full merge flow tested in integration)

- [ ] **Step 7: Commit**

```bash
git add internal/builtin/commandcenter/cc_view.go internal/builtin/commandcenter/cc_keys_detail.go internal/builtin/commandcenter/commandcenter.go
git commit -m "Add sources section and unmerge keybinding to detail view"
```

---

## Task 9: UX polish — search by display_id, flash messages

**Files:**
- Modify: `internal/builtin/commandcenter/commandcenter.go`
- Test: `internal/builtin/commandcenter/commandcenter_test.go`

- [ ] **Step 1: Write failing test for search matching display_id**

```go
func TestSearchFilterMatchesDisplayID(t *testing.T) {
    p := testPluginWithCC(t)
    p.cc.Todos = []db.Todo{
        {ID: "a", DisplayID: 183, Title: "Send report", Status: db.StatusBacklog},
        {ID: "b", DisplayID: 42, Title: "Review PR", Status: db.StatusBacklog},
    }
    p.searchInput.SetValue("183")
    filtered := p.filteredTodos()
    if len(filtered) != 1 || filtered[0].ID != "a" {
        t.Errorf("expected search '183' to match todo #183, got %d results", len(filtered))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/builtin/commandcenter/ -run TestSearchFilterMatchesDisplayID -v`
Expected: FAIL — search doesn't match display IDs

- [ ] **Step 3: Update filteredTodos to match display_id**

In `commandcenter.go`, in the search filter section of `filteredTodos`:

```go
for _, t := range result {
    titleMatch := strings.Contains(strings.ToLower(flattenTitle(t.Title)), lower)
    idMatch := query == fmt.Sprintf("%d", t.DisplayID)
    if titleMatch || idMatch {
        filtered = append(filtered, t)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/builtin/commandcenter/ -run TestSearchFilterMatchesDisplayID -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/builtin/commandcenter/commandcenter.go internal/builtin/commandcenter/commandcenter_test.go
git commit -m "Search filter matches display_id, flash message includes todo number"
```

---

## Task 10: Integration test — full merge cycle

**Files:**
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write integration test for full merge → unmerge cycle**

```go
func TestFullMergeCycle(t *testing.T) {
    database := setupTestDB(t)
    defer database.Close()

    // Create two originals
    a := Todo{ID: "a", DisplayID: 1, Title: "Do X", Status: StatusBacklog, Source: "manual"}
    b := Todo{ID: "b", DisplayID: 2, Title: "Do X tomorrow", Status: StatusBacklog, Source: "manual", Due: "2026-03-20"}
    DBInsertTodo(database, a)
    DBInsertTodo(database, b)

    // Create synthesis
    s := Todo{ID: "s1", DisplayID: 3, Title: "Do X by tomorrow", Status: StatusBacklog, Source: "merge", Due: "2026-03-20"}
    DBInsertTodo(database, s)
    DBInsertMerge(database, "s1", "a", "same task")
    DBInsertMerge(database, "s1", "b", "same task")

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

    // Unmerge b
    DBSetMergeVetoed(database, "s1", "b", true)
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
}
```

- [ ] **Step 2: Run integration test**

Run: `go test ./internal/db/ -run TestFullMergeCycle -v`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 4: Commit**

```bash
git add internal/db/db_test.go
git commit -m "Add integration test for full merge-unmerge cycle"
```

---

## Summary

| Task | What | Depends On |
|------|------|-----------|
| 1 | Schema: `cc_todo_merges` table | — |
| 2 | Merge CRUD operations | 1 |
| 3 | Visibility filtering | 1, 2 |
| 4 | Synthesis LLM module | — |
| 5 | Enrichment prompt expansion | 3 |
| 6 | Enrich handler merge flow | 3, 4, 5 |
| 7 | Refresh path dedup | 3, 4 |
| 8 | Detail view + unmerge UI | 2, 3 |
| 9 | UX polish (search, flash) | 3 |
| 10 | Integration test | 1-3 |

**Parallelizable:** Tasks 1+4 can run in parallel (no shared files). Tasks 8+9 can run in parallel after their deps.
