package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Fixture transcripts
// ---------------------------------------------------------------------------

const fixtureGranolaTranscript = `
Aaron: I think we should move the API gateway to Go. The Node.js version is hitting memory limits.
Zach: I agree. We've been discussing the API gateway migration for weeks now. Let's just do it.
Aaron: Great. Decision made -- we're migrating the API gateway to Go. I'll start with the auth middleware.
Zach: What about the rate limiter? That's still an open question.
Aaron: Right, the rate limiter design is blocking on benchmarks from the Go prototype. Let's revisit next week.
`

const fixtureSlackThread = `
Aaron: Heads up -- the onboarding flow redesign is going live next Monday.
Sarah: Are we keeping the existing welcome email or replacing it?
Aaron: Replacing it. The new version tested 30% better in the A/B test.
Aaron: Decision: we're going with the new welcome email template.
Sarah: There's still the open question about the mobile deep link. Has anyone tested it on Android?
`

const fixtureGmailBody = `
Hi Aaron,

Following up on our discussion about the Q2 budget allocation. I believe we should increase the infrastructure budget by 15% given the recent growth numbers.

The data pipeline costs are a concern -- we're spending more than planned on Snowflake credits. This is an open thread that needs resolution before the next board meeting.

Best,
Finance Team
`

// ---------------------------------------------------------------------------
// Mock LLM for extraction tests
// ---------------------------------------------------------------------------

// mockExtractionLLM returns a pre-canned JSON extraction result.
type mockExtractionLLM struct {
	response string
	err      error
}

func (m *mockExtractionLLM) Complete(ctx context.Context, prompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// newMockLLM creates a mock LLM that returns a valid extraction JSON response.
func newMockLLM() llm.LLM {
	result := map[string]interface{}{
		"topics": []map[string]string{
			{"name": "API Gateway Migration", "description": "Migrating the API gateway from Node.js to Go"},
			{"name": "Rate Limiter Design", "description": "Designing the rate limiter for the new Go API gateway"},
		},
		"decisions": []map[string]interface{}{
			{
				"title":         "Migrate API gateway to Go",
				"description":   "Decision to migrate the API gateway from Node.js to Go due to memory limits",
				"alternatives":  "Keep Node.js with memory optimization",
				"reasoning":     "Node.js version hitting memory limits",
				"participants":  []string{"Aaron", "Zach"},
				"aaron_present": true,
				"decided_at":    "2026-04-10T10:00:00Z",
			},
		},
		"positions": []map[string]interface{}{
			{
				"holder":     "Aaron",
				"position":   "We should move the API gateway to Go",
				"topic_name": "API Gateway Migration",
				"stated_at":  "2026-04-10T10:00:00Z",
			},
			{
				"holder":     "Zach",
				"position":   "Agrees with migrating to Go",
				"topic_name": "API Gateway Migration",
				"stated_at":  "2026-04-10T10:00:00Z",
			},
		},
		"open_threads": []map[string]interface{}{
			{
				"description":    "Rate limiter design blocking on Go prototype benchmarks",
				"blocking_on":    "Benchmarks from Go prototype",
				"topic_name":     "Rate Limiter Design",
				"status":         "open",
				"first_raised_by": "Aaron",
			},
		},
	}
	data, _ := json.Marshal(result)
	return &mockExtractionLLM{response: string(data)}
}

// setupExtractionDB creates a test database with knowledge tables.
// Since migrations aren't implemented yet, this creates the tables inline.
func setupExtractionDB(t *testing.T) *sql.DB {
	t.Helper()
	database := openTestDB(t)

	// Try to run real migrations first
	p := New()
	migrations := p.Migrations()
	if len(migrations) > 0 {
		if err := plugin.RunMigrations(database, p.Slug(), migrations); err != nil {
			t.Fatalf("RunMigrations: %v", err)
		}
	} else {
		// Migrations not implemented yet -- create tables inline for extraction tests
		t.Fatal("knowledge plugin migrations not implemented; extraction tests require tables to exist")
	}
	return database
}

// ---------------------------------------------------------------------------
// Extraction tests
// ---------------------------------------------------------------------------

func TestExtract_GranolaProducesArtifacts(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	result, err := Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if result == nil {
		t.Fatal("Extract should return a non-nil result")
	}
	if len(result.Topics) == 0 {
		t.Error("extraction should produce at least one topic")
	}
	if len(result.Decisions) == 0 {
		t.Error("extraction should produce at least one decision")
	}
	if len(result.Positions) == 0 {
		t.Error("extraction should produce at least one position")
	}
	if len(result.OpenThreads) == 0 {
		t.Error("extraction should produce at least one open thread")
	}

	// Verify artifacts were written to the database
	var topicCount int
	database.QueryRow("SELECT COUNT(*) FROM knowledge_topics").Scan(&topicCount)
	if topicCount == 0 {
		t.Error("topics should be written to the database")
	}

	var decisionCount int
	database.QueryRow("SELECT COUNT(*) FROM knowledge_decisions").Scan(&decisionCount)
	if decisionCount == 0 {
		t.Error("decisions should be written to the database")
	}
}

func TestExtract_SlackProducesArtifacts(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	result, err := Extract(context.Background(), database, model, "slack-msg-456", "slack", fixtureSlackThread, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if result == nil {
		t.Fatal("Extract should return a non-nil result")
	}
}

func TestExtract_GmailProducesArtifacts(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	result, err := Extract(context.Background(), database, model, "gmail-thread-789", "gmail", fixtureGmailBody, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if result == nil {
		t.Fatal("Extract should return a non-nil result")
	}
}

// ---------------------------------------------------------------------------
// Idempotence tests
// ---------------------------------------------------------------------------

func TestExtract_IdempotenceNoDuplicates(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	// Run extraction twice on the same source_ref
	_, err := Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, nil)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}

	_, err = Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, nil)
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}

	// Decisions should not be duplicated
	var decisionCount int
	database.QueryRow("SELECT COUNT(*) FROM knowledge_decisions WHERE source_ref = 'meeting-123'").Scan(&decisionCount)
	if decisionCount > 1 {
		t.Errorf("expected at most 1 decision for source_ref 'meeting-123', got %d", decisionCount)
	}

	// Positions should not be duplicated
	var positionCount int
	database.QueryRow("SELECT COUNT(*) FROM knowledge_positions WHERE source_ref = 'meeting-123'").Scan(&positionCount)
	if positionCount > 2 {
		// The mock returns 2 positions (Aaron and Zach), so max 2
		t.Errorf("expected at most 2 positions for source_ref 'meeting-123', got %d", positionCount)
	}
}

func TestExtract_TopicMentionCountUpdated(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	// First extraction
	_, err := Extract(context.Background(), database, model, "meeting-100", "granola", fixtureGranolaTranscript, nil)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}

	// Get existing topics for second extraction
	var existingTopics []string
	rows, _ := database.Query("SELECT name FROM knowledge_topics")
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		existingTopics = append(existingTopics, name)
	}

	// Second extraction from a different source referencing the same topics
	_, err = Extract(context.Background(), database, model, "meeting-200", "granola", fixtureGranolaTranscript, existingTopics)
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}

	// Topic mention_count should be > 1 after two mentions
	var mentionCount int
	database.QueryRow("SELECT mention_count FROM knowledge_topics WHERE name = 'API Gateway Migration'").Scan(&mentionCount)
	if mentionCount <= 1 {
		t.Errorf("expected mention_count > 1 after two extractions, got %d", mentionCount)
	}
}

func TestExtract_TopicDeduplication(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	// Extract twice with different source refs
	Extract(context.Background(), database, model, "meeting-100", "granola", fixtureGranolaTranscript, nil)
	Extract(context.Background(), database, model, "meeting-200", "granola", fixtureGranolaTranscript, nil)

	// Should have exactly 2 topics (the mock returns 2), not 4
	var topicCount int
	database.QueryRow("SELECT COUNT(*) FROM knowledge_topics").Scan(&topicCount)
	if topicCount > 2 {
		t.Errorf("expected at most 2 unique topics, got %d (deduplication should prevent duplicates)", topicCount)
	}
}

// ---------------------------------------------------------------------------
// Edge inference tests
// ---------------------------------------------------------------------------

func TestExtract_EvolvesEdgeCreated(t *testing.T) {
	database := setupExtractionDB(t)

	// Insert a prior position by Aaron on the same topic
	_, err := database.Exec(`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		VALUES ('topic-1', 'API Gateway Migration', 'Migration topic', '2026-04-01T00:00:00Z', '2026-04-01T00:00:00Z', 1)`)
	if err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	_, err = database.Exec(`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		VALUES ('pos-old', 'Aaron', 'We should consider migrating to Go', 'topic-1', 'granola', 'meeting-050', '2026-04-01T00:00:00Z', '2026-04-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert prior position: %v", err)
	}

	// Now extract a new position by Aaron on the same topic
	model := newMockLLM()
	_, err = Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, []string{"API Gateway Migration"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// There should be an "evolves" edge from the new position to the old one
	var edgeCount int
	database.QueryRow(`SELECT COUNT(*) FROM knowledge_edges WHERE relationship = 'evolves' AND to_id = 'pos-old'`).Scan(&edgeCount)
	if edgeCount == 0 {
		t.Error("expected an 'evolves' edge from new position to prior position 'pos-old'")
	}
}

func TestExtract_NoEvolvesEdgeForDifferentHolder(t *testing.T) {
	database := setupExtractionDB(t)

	// Insert a prior position by Zach (not Aaron)
	_, err := database.Exec(`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		VALUES ('topic-1', 'API Gateway Migration', 'Migration topic', '2026-04-01T00:00:00Z', '2026-04-01T00:00:00Z', 1)`)
	if err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	_, err = database.Exec(`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		VALUES ('pos-zach', 'Zach', 'I prefer staying with Node.js', 'topic-1', 'granola', 'meeting-050', '2026-04-01T00:00:00Z', '2026-04-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert Zach position: %v", err)
	}

	// Create a mock that only returns an Aaron position
	aaronOnlyResult := map[string]interface{}{
		"topics":       []map[string]string{{"name": "API Gateway Migration", "description": "Migration"}},
		"decisions":    []interface{}{},
		"positions":    []map[string]interface{}{{"holder": "Aaron", "position": "We should move to Go", "topic_name": "API Gateway Migration", "stated_at": "2026-04-10T10:00:00Z"}},
		"open_threads": []interface{}{},
	}
	data, _ := json.Marshal(aaronOnlyResult)
	model := &mockExtractionLLM{response: string(data)}

	_, err = Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, []string{"API Gateway Migration"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Should NOT have an evolves edge from Aaron's position to Zach's position
	var edgeCount int
	database.QueryRow(`SELECT COUNT(*) FROM knowledge_edges WHERE relationship = 'evolves' AND to_id = 'pos-zach'`).Scan(&edgeCount)
	if edgeCount > 0 {
		t.Error("should not create an 'evolves' edge between positions by different holders")
	}
}

func TestExtract_NoEvolvesEdgeForFirstPosition(t *testing.T) {
	database := setupExtractionDB(t)
	model := newMockLLM()

	// Extract on a fresh database with no prior positions
	_, err := Extract(context.Background(), database, model, "meeting-123", "granola", fixtureGranolaTranscript, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// No evolves edges should exist when there are no prior positions
	var edgeCount int
	database.QueryRow(`SELECT COUNT(*) FROM knowledge_edges WHERE relationship = 'evolves'`).Scan(&edgeCount)
	if edgeCount > 0 {
		t.Error("no 'evolves' edges should exist when extracting the first positions on a topic")
	}
}
