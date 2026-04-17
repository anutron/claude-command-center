package knowledge

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/anutron/claude-command-center/internal/llm"
)

// ExtractionResult holds the parsed extraction output from the LLM.
type ExtractionResult struct {
	Topics      []ExtractedTopic
	Decisions   []ExtractedDecision
	Positions   []ExtractedPosition
	OpenThreads []ExtractedOpenThread
}

// ExtractedTopic is a topic extracted from source material.
type ExtractedTopic struct {
	Name        string
	Description string
}

// ExtractedDecision is a decision extracted from source material.
type ExtractedDecision struct {
	Title        string
	Description  string
	Alternatives string
	Reasoning    string
	Participants []string
	AaronPresent bool
	DecidedAt    string
}

// ExtractedPosition is a position extracted from source material.
type ExtractedPosition struct {
	Holder    string
	Position  string
	TopicName string
	StatedAt  string
}

// ExtractedOpenThread is an open thread extracted from source material.
type ExtractedOpenThread struct {
	Description   string
	BlockingOn    string
	TopicName     string
	Status        string
	FirstRaisedBy string
}

// Extract processes source content through the LLM and writes knowledge
// artifacts to the database. Returns an error if extraction fails.
//
// Stub: returns an error – Stage 4 will implement the real extraction.
func Extract(ctx context.Context, database *sql.DB, model llm.LLM, sourceRef, sourceType, content string, existingTopics []string) (*ExtractionResult, error) {
	return nil, fmt.Errorf("extraction not implemented")
}
