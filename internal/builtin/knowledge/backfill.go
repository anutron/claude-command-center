package knowledge

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/anutron/claude-command-center/internal/llm"
)

// BackfillState tracks per-source backfill progress.
type BackfillState struct {
	Source      string
	LastOffset  string // last processed timestamp or cursor
	Completed   bool
}

// RunBackfill performs the one-time 30-day backfill of knowledge extraction.
// It processes historical source content and runs extraction on each item.
// Progress is tracked per source for resumability.
//
// Stub: returns an error – Stage 9 will implement the real backfill.
func RunBackfill(ctx context.Context, database *sql.DB, model llm.LLM) error {
	return fmt.Errorf("backfill not implemented")
}

// IsBackfillComplete checks whether the backfill has already completed.
//
// Stub: returns false – Stage 9 will implement the real check.
func IsBackfillComplete(database *sql.DB) (bool, error) {
	return false, fmt.Errorf("backfill state check not implemented")
}

// ResetBackfill clears the backfill state so it can be re-triggered.
//
// Stub: returns an error – Stage 9 will implement.
func ResetBackfill(database *sql.DB) error {
	return fmt.Errorf("backfill reset not implemented")
}
