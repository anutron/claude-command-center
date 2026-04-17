package knowledge

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// RunDriftDetection analyzes Aaron's recent positions for evidence of
// stance shifts using the LLM. Writes drift insights to
// knowledge_surfaced_insights when shifts are detected.
//
// Stub: returns an error – Stage 8 will implement the real analysis.
func RunDriftDetection(ctx context.Context, database *sql.DB, model llm.LLM, bus plugin.EventBus) error {
	return fmt.Errorf("drift detection not implemented")
}
