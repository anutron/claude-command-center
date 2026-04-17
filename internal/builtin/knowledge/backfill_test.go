package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/anutron/claude-command-center/internal/plugin"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupBackfillDB creates a test database with knowledge tables for backfill testing.
func setupBackfillDB(t *testing.T) *sql.DB {
	t.Helper()
	database := openTestDB(t)

	p := New()
	migrations := p.Migrations()
	if len(migrations) == 0 {
		t.Fatal("knowledge plugin migrations not implemented; backfill tests require tables to exist")
	}
	if err := plugin.RunMigrations(database, p.Slug(), migrations); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return database
}

// newBackfillMockLLM creates a mock LLM that returns minimal extraction results.
func newBackfillMockLLM() *mockExtractionLLM {
	result := map[string]interface{}{
		"topics": []map[string]string{
			{"name": "Backfill Topic", "description": "A topic found during backfill"},
		},
		"decisions":    []interface{}{},
		"positions":    []interface{}{},
		"open_threads": []interface{}{},
	}
	data, _ := json.Marshal(result)
	return &mockExtractionLLM{response: string(data)}
}

// ---------------------------------------------------------------------------
// Backfill tests: 30-day window
// ---------------------------------------------------------------------------

func TestBackfill_ProcessesLast30Days(t *testing.T) {
	database := setupBackfillDB(t)
	model := newBackfillMockLLM()

	err := RunBackfill(context.Background(), database, model)
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}

	// After successful backfill, the backfill state should indicate completion
	complete, err := IsBackfillComplete(database)
	if err != nil {
		t.Fatalf("IsBackfillComplete: %v", err)
	}
	if !complete {
		t.Error("backfill should be marked as complete after successful run")
	}
}

// ---------------------------------------------------------------------------
// Backfill tests: resumability
// ---------------------------------------------------------------------------

func TestBackfill_ResumesAfterPartialFailure(t *testing.T) {
	database := setupBackfillDB(t)

	// Simulate a partial backfill by marking one source as completed
	// but leaving others incomplete
	_, err := database.Exec(`UPDATE knowledge_backfill_state SET last_offset = '2026-04-01T00:00:00Z' WHERE source = 'granola'`)
	if err != nil {
		// This might fail if the backfill_state table doesn't have a source column yet,
		// which is fine -- the test should fail because migrations aren't implemented.
		t.Fatalf("simulate partial backfill: %v", err)
	}

	model := newBackfillMockLLM()

	// Running backfill should pick up where it left off
	err = RunBackfill(context.Background(), database, model)
	if err != nil {
		t.Fatalf("RunBackfill (resume): %v", err)
	}

	complete, err := IsBackfillComplete(database)
	if err != nil {
		t.Fatalf("IsBackfillComplete: %v", err)
	}
	if !complete {
		t.Error("backfill should be complete after resumed run finishes")
	}
}

// ---------------------------------------------------------------------------
// Backfill tests: one-shot (does not re-run)
// ---------------------------------------------------------------------------

func TestBackfill_DoesNotReRunAfterCompletion(t *testing.T) {
	database := setupBackfillDB(t)
	model := newBackfillMockLLM()

	// Run backfill the first time
	err := RunBackfill(context.Background(), database, model)
	if err != nil {
		t.Fatalf("first RunBackfill: %v", err)
	}

	// Verify it completed
	complete, err := IsBackfillComplete(database)
	if err != nil {
		t.Fatalf("IsBackfillComplete: %v", err)
	}
	if !complete {
		t.Fatal("backfill should be complete")
	}

	// Run backfill again -- should be a no-op
	// We use a counting mock to verify no LLM calls are made
	countingModel := &countingLLM{inner: model}
	err = RunBackfill(context.Background(), database, countingModel)
	if err != nil {
		t.Fatalf("second RunBackfill: %v", err)
	}

	if countingModel.calls > 0 {
		t.Errorf("backfill should not make LLM calls on re-run after completion, but made %d calls", countingModel.calls)
	}
}

// countingLLM wraps an LLM and counts how many calls are made.
type countingLLM struct {
	inner *mockExtractionLLM
	calls int
}

func (c *countingLLM) Complete(ctx context.Context, prompt string) (string, error) {
	c.calls++
	return c.inner.Complete(ctx, prompt)
}
