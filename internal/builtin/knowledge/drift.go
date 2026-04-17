package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/llm"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// driftPosition represents an Aaron position eligible for drift evaluation.
type driftPosition struct {
	id       string
	holder   string
	position string
	topicID  string
	statedAt string
}

// driftEvidence represents a newer decision or position on the same topic.
type driftEvidence struct {
	kind      string // "decision" or "position"
	text      string
	timestamp string
}

// driftResult is the expected structure from the LLM response.
type driftResult struct {
	PositionID       string `json:"position_id"`
	OriginalPosition string `json:"original_position"`
	EvidenceOfShift  string `json:"evidence_of_shift"`
	ShiftedTo        string `json:"shifted_to"`
}

// RunDriftDetection analyzes Aaron's recent positions for evidence of
// stance shifts using the LLM. Writes drift insights to
// knowledge_surfaced_insights when shifts are detected.
//
// Steps:
//  1. Query Aaron's positions from the last 60 days
//  2. For each position, gather newer decisions/positions on the same topic
//  3. Skip positions with no newer evidence
//  4. Build a Sonnet prompt and parse the response
//  5. Write drift insights for positions where shift is detected
//  6. Publish knowledge.insights.updated on the bus
func RunDriftDetection(ctx context.Context, database *sql.DB, model llm.LLM, bus plugin.EventBus) error {
	// 1. Query Aaron's positions from the last 60 days.
	cutoff := time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
	rows, err := database.Query(
		`SELECT id, holder, position, topic_id, stated_at
		 FROM knowledge_positions
		 WHERE holder = 'Aaron' AND stated_at > ?`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("querying positions for drift: %w", err)
	}

	var positions []driftPosition
	for rows.Next() {
		var p driftPosition
		var topicID sql.NullString
		if err := rows.Scan(&p.id, &p.holder, &p.position, &topicID, &p.statedAt); err != nil {
			log.Printf("drift: scanning position row: %v", err)
			continue
		}
		if topicID.Valid {
			p.topicID = topicID.String
		}
		positions = append(positions, p)
	}
	rows.Close()

	if len(positions) == 0 {
		return nil
	}

	// 2. For each position, gather newer evidence and build prompts.
	changed := false
	validPositionIDs := map[string]bool{}

	for _, pos := range positions {
		validPositionIDs[pos.id] = true

		// Skip if position has no topic (cannot find related evidence).
		if pos.topicID == "" {
			continue
		}

		evidence := gatherDriftEvidence(database, pos.topicID, pos.statedAt)
		if len(evidence) == 0 {
			continue
		}

		// 3. Build prompt and call LLM.
		prompt := buildDriftPrompt(pos, evidence)
		response, err := model.Complete(ctx, prompt)
		if err != nil {
			log.Printf("drift: LLM call for position %s: %v", pos.id, err)
			continue
		}

		// 4. Parse response.
		results, err := parseDriftResponse(response)
		if err != nil {
			log.Printf("drift: parsing LLM response for position %s: %v", pos.id, err)
			continue
		}

		// 5. Write insights for detected drift.
		for _, r := range results {
			// Only write insights for positions we actually queried.
			if r.PositionID != pos.id {
				continue
			}

			insightID := fmt.Sprintf("drift-position-%s", r.PositionID)
			title := fmt.Sprintf("Position may have shifted: %s", truncate(r.OriginalPosition, 80))
			body := fmt.Sprintf("Original position: %s\n\nEvidence of shift: %s\n\nShifted to: %s",
				r.OriginalPosition, r.EvidenceOfShift, r.ShiftedTo)
			sourceRefs, _ := json.Marshal([]string{r.PositionID})

			if err := upsertDriftInsight(database, insightID, title, body, string(sourceRefs)); err != nil {
				log.Printf("drift: upserting insight %s: %v", insightID, err)
				continue
			}
			changed = true
		}
	}

	// 6. Publish event if changes occurred.
	if changed && bus != nil {
		bus.Publish(plugin.Event{
			Source: "knowledge",
			Topic:  "knowledge.insights.updated",
		})
	}

	return nil
}

// gatherDriftEvidence collects newer decisions and positions on the same topic
// that were created after the given timestamp.
func gatherDriftEvidence(database *sql.DB, topicID, afterTimestamp string) []driftEvidence {
	var evidence []driftEvidence

	// Newer decisions on the same topic (via topic_id on positions that reference decisions).
	// Decisions don't have a direct topic_id, so we look for decisions whose decided_at
	// is after the position's stated_at. We join through positions on the same topic.
	decRows, err := database.Query(
		`SELECT DISTINCT d.title, d.description, d.decided_at
		 FROM knowledge_decisions d
		 WHERE d.decided_at > ?
		 ORDER BY d.decided_at ASC`,
		afterTimestamp,
	)
	if err == nil {
		for decRows.Next() {
			var title, desc, decidedAt string
			if err := decRows.Scan(&title, &desc, &decidedAt); err != nil {
				continue
			}
			evidence = append(evidence, driftEvidence{
				kind:      "decision",
				text:      fmt.Sprintf("%s – %s", title, desc),
				timestamp: decidedAt,
			})
		}
		decRows.Close()
	}

	// Newer positions on the same topic.
	posRows, err := database.Query(
		`SELECT holder, position, stated_at
		 FROM knowledge_positions
		 WHERE topic_id = ? AND stated_at > ?
		 ORDER BY stated_at ASC`,
		topicID, afterTimestamp,
	)
	if err == nil {
		for posRows.Next() {
			var holder, position, statedAt string
			if err := posRows.Scan(&holder, &position, &statedAt); err != nil {
				continue
			}
			evidence = append(evidence, driftEvidence{
				kind:      "position",
				text:      fmt.Sprintf("%s: %s", holder, position),
				timestamp: statedAt,
			})
		}
		posRows.Close()
	}

	return evidence
}

// buildDriftPrompt constructs the Sonnet prompt for drift detection.
func buildDriftPrompt(pos driftPosition, evidence []driftEvidence) string {
	var b strings.Builder

	b.WriteString("You are analyzing whether a stated position has shifted based on newer evidence.\n\n")
	b.WriteString("## Original position\n\n")
	b.WriteString(fmt.Sprintf("- **Position ID**: %s\n", pos.id))
	b.WriteString(fmt.Sprintf("- **Stated by**: %s\n", pos.holder))
	b.WriteString(fmt.Sprintf("- **Position**: %s\n", pos.position))
	b.WriteString(fmt.Sprintf("- **Stated at**: %s\n\n", pos.statedAt))

	b.WriteString("## Newer decisions and positions\n\n")
	for _, e := range evidence {
		b.WriteString(fmt.Sprintf("- [%s, %s] %s\n", e.kind, e.timestamp, e.text))
	}

	b.WriteString("\n## Instructions\n\n")
	b.WriteString("Has the plan or stance shifted away from this position? ")
	b.WriteString("If yes, return a JSON array with one object containing:\n")
	b.WriteString("- position_id: the position ID from above\n")
	b.WriteString("- original_position: the original position text\n")
	b.WriteString("- evidence_of_shift: describe the evidence of shift\n")
	b.WriteString("- shifted_to: what the stance has shifted to\n\n")
	b.WriteString("If no drift is detected, return an empty JSON array: []\n\n")
	b.WriteString("Return ONLY the JSON array, no other text.")

	return b.String()
}

// parseDriftResponse parses the LLM's JSON response into drift results.
func parseDriftResponse(response string) ([]driftResult, error) {
	response = strings.TrimSpace(response)

	// Handle NO_DRIFT response.
	if response == "NO_DRIFT" {
		return nil, nil
	}

	// Try to extract JSON array from the response.
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start >= 0 && end > start {
		response = response[start : end+1]
	}

	var results []driftResult
	if err := json.Unmarshal([]byte(response), &results); err != nil {
		return nil, fmt.Errorf("parsing drift JSON: %w", err)
	}

	return results, nil
}

// upsertDriftInsight inserts a new drift insight or updates an existing one.
// If the insight was previously dismissed, it is left unchanged.
func upsertDriftInsight(database *sql.DB, id, title, body, sourceRefs string) error {
	var existingID string
	var dismissedAt *string
	err := database.QueryRow(
		"SELECT id, dismissed_at FROM knowledge_surfaced_insights WHERE id = ?", id,
	).Scan(&existingID, &dismissedAt)

	if err == sql.ErrNoRows {
		_, err = database.Exec(
			`INSERT INTO knowledge_surfaced_insights (id, type, title, body, source_refs, priority, surfaced_at)
			 VALUES (?, 'drift_detection', ?, ?, ?, 40, datetime('now'))`,
			id, title, body, sourceRefs,
		)
		return err
	}
	if err != nil {
		return err
	}

	// Existing: if dismissed, leave it alone.
	if dismissedAt != nil {
		return nil
	}

	// Update non-dismissed insight.
	_, err = database.Exec(
		`UPDATE knowledge_surfaced_insights SET title = ?, body = ?, source_refs = ?, surfaced_at = datetime('now') WHERE id = ?`,
		title, body, sourceRefs, id,
	)
	return err
}

// truncate is defined in extraction.go — reused here.
