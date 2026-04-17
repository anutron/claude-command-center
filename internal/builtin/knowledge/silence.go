package knowledge

import (
	"database/sql"
	"fmt"

	"github.com/anutron/claude-command-center/internal/plugin"
)

// SilenceConfig holds the thresholds for silence alert detection.
type SilenceConfig struct {
	TopicSilenceDays  int // default: 10
	ThreadSilenceDays int // default: 7
}

// DefaultSilenceConfig returns the default silence alert configuration.
func DefaultSilenceConfig() SilenceConfig {
	return SilenceConfig{
		TopicSilenceDays:  10,
		ThreadSilenceDays: 7,
	}
}

// RunSilenceAnalysis scans knowledge tables for topics and open threads
// that have gone quiet and writes silence alert insights.
//
// Stub: returns an error – Stage 7 will implement the real analysis.
func RunSilenceAnalysis(database *sql.DB, bus plugin.EventBus, cfg SilenceConfig) error {
	return fmt.Errorf("silence analysis not implemented")
}
