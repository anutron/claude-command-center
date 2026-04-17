package knowledge

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
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

// rawExtractionResponse mirrors the JSON structure returned by the LLM.
type rawExtractionResponse struct {
	Topics []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"topics"`
	Decisions []struct {
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		Alternatives string   `json:"alternatives"`
		Reasoning    string   `json:"reasoning"`
		Participants []string `json:"participants"`
		AaronPresent bool     `json:"aaron_present"`
		DecidedAt    string   `json:"decided_at"`
	} `json:"decisions"`
	Positions []struct {
		Holder    string `json:"holder"`
		Position  string `json:"position"`
		TopicName string `json:"topic_name"`
		StatedAt  string `json:"stated_at"`
	} `json:"positions"`
	OpenThreads []struct {
		Description   string `json:"description"`
		BlockingOn    string `json:"blocking_on"`
		TopicName     string `json:"topic_name"`
		Status        string `json:"status"`
		FirstRaisedBy string `json:"first_raised_by"`
	} `json:"open_threads"`
}

// Extract processes source content through the LLM and writes knowledge
// artifacts to the database. Returns the extracted artifacts or an error
// if extraction fails.
//
// The function is idempotent: calling it multiple times with the same
// sourceRef does not create duplicate artifacts.
func Extract(ctx context.Context, database *sql.DB, model llm.LLM, sourceRef, sourceType, content string, existingTopics []string) (*ExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &ExtractionResult{}, nil
	}

	// Build the extraction prompt.
	prompt := buildExtractionPrompt(sourceType, sourceRef, content, existingTopics)

	// Call the LLM.
	raw, err := model.Complete(llm.WithOperation(ctx, "knowledge-extract"), prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction: %w", err)
	}

	// Parse the JSON response.
	raw = cleanJSON(raw)
	var resp rawExtractionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("parsing extraction JSON: %w (raw: %s)", err, truncate(raw, 200))
	}

	now := db.FormatTime(time.Now())

	// Convert to result types.
	result := &ExtractionResult{}
	for _, t := range resp.Topics {
		result.Topics = append(result.Topics, ExtractedTopic{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	for _, d := range resp.Decisions {
		result.Decisions = append(result.Decisions, ExtractedDecision{
			Title:        d.Title,
			Description:  d.Description,
			Alternatives: d.Alternatives,
			Reasoning:    d.Reasoning,
			Participants: d.Participants,
			AaronPresent: d.AaronPresent,
			DecidedAt:    d.DecidedAt,
		})
	}
	for _, p := range resp.Positions {
		result.Positions = append(result.Positions, ExtractedPosition{
			Holder:    p.Holder,
			Position:  p.Position,
			TopicName: p.TopicName,
			StatedAt:  p.StatedAt,
		})
	}
	for _, ot := range resp.OpenThreads {
		result.OpenThreads = append(result.OpenThreads, ExtractedOpenThread{
			Description:   ot.Description,
			BlockingOn:    ot.BlockingOn,
			TopicName:     ot.TopicName,
			Status:        ot.Status,
			FirstRaisedBy: ot.FirstRaisedBy,
		})
	}

	// Write artifacts to the database.
	if err := writeArtifacts(database, result, sourceRef, sourceType, now); err != nil {
		return result, fmt.Errorf("writing artifacts: %w", err)
	}

	return result, nil
}

// buildExtractionPrompt constructs the LLM prompt for knowledge extraction.
func buildExtractionPrompt(sourceType, sourceRef, content string, existingTopics []string) string {
	var sb strings.Builder

	sb.WriteString(`Analyze the following source content and extract structured knowledge artifacts. Return ONLY a JSON object with four arrays: topics, decisions, positions, and open_threads.

Guidelines:
- Only extract substantive artifacts, not every passing mention
- Topics are recurring subjects discussed in the content
- Decisions are discrete choices made during the conversation
- Positions are stated stances someone took on a topic
- Open threads are things raised but not resolved

`)

	if len(existingTopics) > 0 {
		sb.WriteString("IMPORTANT: The following topic names already exist in the knowledge base. Reuse these exact names (case-insensitive) when the same subject is discussed. Do NOT create a new topic name if an existing one covers the same subject.\n\nExisting topics:\n")
		for _, t := range existingTopics {
			sb.WriteString("- ")
			sb.WriteString(t)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Source type: %s\nSource reference: %s\n\n", sourceType, sourceRef))

	sb.WriteString(`Return ONLY valid JSON with this exact structure, no other text:
{
  "topics": [
    {"name": "string (reuse existing name if applicable)", "description": "string"}
  ],
  "decisions": [
    {
      "title": "string",
      "description": "string",
      "alternatives": "string or null",
      "reasoning": "string or null",
      "participants": ["string"],
      "aaron_present": true,
      "decided_at": "RFC3339 timestamp"
    }
  ],
  "positions": [
    {
      "holder": "string (e.g. 'Aaron', 'Zach')",
      "position": "string",
      "topic_name": "string or null (references a topic from the topics array)",
      "stated_at": "RFC3339 timestamp"
    }
  ],
  "open_threads": [
    {
      "description": "string",
      "blocking_on": "string or null",
      "topic_name": "string or null",
      "status": "open",
      "first_raised_by": "string"
    }
  ]
}

If no artifacts of a particular type are found, return an empty array for that type.

<source_content>
`)
	sb.WriteString(content)
	sb.WriteString("\n</source_content>")

	return sb.String()
}

// writeArtifacts persists extracted artifacts to the database with idempotence.
func writeArtifacts(database *sql.DB, result *ExtractionResult, sourceRef, sourceType, now string) error {
	// Build a topic name -> ID map for linking positions and threads.
	topicIDs := make(map[string]string) // lowercase name -> id

	// Upsert topics.
	for _, t := range result.Topics {
		id, err := upsertTopic(database, t, now)
		if err != nil {
			log.Printf("knowledge: upsert topic %q: %v", t.Name, err)
			continue
		}
		topicIDs[strings.ToLower(t.Name)] = id
	}

	// Insert decisions with idempotence (skip if source_ref + title already exists).
	for _, d := range result.Decisions {
		if err := insertDecision(database, d, sourceRef, sourceType, now); err != nil {
			log.Printf("knowledge: insert decision %q: %v", d.Title, err)
		}
	}

	// Insert positions with idempotence and edge inference.
	for _, p := range result.Positions {
		topicID := topicIDs[strings.ToLower(p.TopicName)]
		if err := insertPosition(database, p, sourceRef, sourceType, topicID, now); err != nil {
			log.Printf("knowledge: insert position by %q: %v", p.Holder, err)
		}
	}

	// Upsert open threads with idempotence.
	for _, ot := range result.OpenThreads {
		topicID := topicIDs[strings.ToLower(ot.TopicName)]
		if err := upsertOpenThread(database, ot, sourceRef, sourceType, topicID, now); err != nil {
			log.Printf("knowledge: upsert open thread %q: %v", ot.Description, err)
		}
	}

	return nil
}

// upsertTopic creates or updates a topic. Returns the topic ID.
func upsertTopic(database *sql.DB, t ExtractedTopic, now string) (string, error) {
	// Look up existing topic by name (case-insensitive, handled by COLLATE NOCASE).
	var existingID string
	err := database.QueryRow(
		`SELECT id FROM knowledge_topics WHERE name = ? COLLATE NOCASE`, t.Name,
	).Scan(&existingID)

	if err == nil {
		// Topic exists: update last_seen and increment mention_count.
		_, err = database.Exec(
			`UPDATE knowledge_topics SET last_seen = ?, mention_count = mention_count + 1 WHERE id = ?`,
			now, existingID,
		)
		return existingID, err
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("looking up topic %q: %w", t.Name, err)
	}

	// New topic: insert.
	id := db.GenID()
	_, err = database.Exec(
		`INSERT INTO knowledge_topics (id, name, description, first_seen, last_seen, mention_count)
		 VALUES (?, ?, ?, ?, ?, 1)`,
		id, t.Name, t.Description, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("inserting topic %q: %w", t.Name, err)
	}
	return id, nil
}

// insertDecision inserts a decision, skipping if one with the same
// source_ref and title already exists (idempotence).
func insertDecision(database *sql.DB, d ExtractedDecision, sourceRef, sourceType, now string) error {
	// Check for existing decision with same source_ref and title.
	var exists int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM knowledge_decisions WHERE source_ref = ? AND title = ?`,
		sourceRef, d.Title,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking existing decision: %w", err)
	}
	if exists > 0 {
		return nil // already extracted, skip
	}

	participants, _ := json.Marshal(d.Participants)
	aaronPresent := 0
	if d.AaronPresent {
		aaronPresent = 1
	}

	decidedAt := d.DecidedAt
	if decidedAt == "" {
		decidedAt = now
	}

	_, err = database.Exec(
		`INSERT INTO knowledge_decisions (id, title, description, alternatives, reasoning, participants, aaron_present, source, source_ref, decided_at, extracted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		db.GenID(), d.Title, d.Description, d.Alternatives, d.Reasoning,
		string(participants), aaronPresent, sourceType, sourceRef, decidedAt, now,
	)
	return err
}

// insertPosition inserts a position, skipping if one with the same
// source_ref, holder, and topic_id already exists. After insertion,
// performs edge inference for evolves relationships.
func insertPosition(database *sql.DB, p ExtractedPosition, sourceRef, sourceType, topicID, now string) error {
	// Check for existing position with same source_ref + holder + topic_id.
	var exists int
	query := `SELECT COUNT(*) FROM knowledge_positions WHERE source_ref = ? AND holder = ?`
	args := []interface{}{sourceRef, p.Holder}
	if topicID != "" {
		query += ` AND topic_id = ?`
		args = append(args, topicID)
	} else {
		query += ` AND topic_id IS NULL`
	}
	err := database.QueryRow(query, args...).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking existing position: %w", err)
	}
	if exists > 0 {
		return nil // already extracted, skip
	}

	statedAt := p.StatedAt
	if statedAt == "" {
		statedAt = now
	}

	posID := db.GenID()
	var topicIDParam interface{}
	if topicID != "" {
		topicIDParam = topicID
	}

	_, err = database.Exec(
		`INSERT INTO knowledge_positions (id, holder, position, topic_id, source, source_ref, stated_at, extracted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		posID, p.Holder, p.Position, topicIDParam, sourceType, sourceRef, statedAt, now,
	)
	if err != nil {
		return fmt.Errorf("inserting position: %w", err)
	}

	// Edge inference: look up prior positions by the same holder on the same topic.
	if topicID != "" {
		if err := inferEvolvesEdge(database, posID, p.Holder, topicID, statedAt, now); err != nil {
			log.Printf("knowledge: edge inference for position %s: %v", posID, err)
		}
	}

	return nil
}

// inferEvolvesEdge creates an "evolves" edge from the new position to the
// most recent prior position by the same holder on the same topic.
func inferEvolvesEdge(database *sql.DB, newPosID, holder, topicID, statedAt, now string) error {
	var priorID string
	err := database.QueryRow(
		`SELECT id FROM knowledge_positions
		 WHERE holder = ? AND topic_id = ? AND id != ? AND stated_at <= ?
		 ORDER BY stated_at DESC LIMIT 1`,
		holder, topicID, newPosID, statedAt,
	).Scan(&priorID)

	if err == sql.ErrNoRows {
		return nil // no prior position, nothing to link
	}
	if err != nil {
		return fmt.Errorf("querying prior positions: %w", err)
	}

	// Insert evolves edge (from new to prior).
	_, err = database.Exec(
		`INSERT OR IGNORE INTO knowledge_edges (from_id, from_type, to_id, to_type, relationship, created_at)
		 VALUES (?, 'position', ?, 'position', 'evolves', ?)`,
		newPosID, priorID, now,
	)
	return err
}

// upsertOpenThread creates or updates an open thread. If a thread with the
// same source_ref and description already exists, updates last_activity_at.
func upsertOpenThread(database *sql.DB, ot ExtractedOpenThread, sourceRef, sourceType, topicID, now string) error {
	// Check for existing thread with same source_ref and description.
	var existingID string
	err := database.QueryRow(
		`SELECT id FROM knowledge_open_threads WHERE source_ref = ? AND description = ?`,
		sourceRef, ot.Description,
	).Scan(&existingID)

	if err == nil {
		// Thread exists: update last_activity_at and optionally status.
		updateQuery := `UPDATE knowledge_open_threads SET last_activity_at = ?`
		updateArgs := []interface{}{now}
		if ot.Status == "resolved" {
			updateQuery += `, status = 'resolved'`
		}
		updateQuery += ` WHERE id = ?`
		updateArgs = append(updateArgs, existingID)
		_, err = database.Exec(updateQuery, updateArgs...)
		return err
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("checking existing thread: %w", err)
	}

	// New thread: insert.
	status := ot.Status
	if status == "" {
		status = "open"
	}

	var topicIDParam interface{}
	if topicID != "" {
		topicIDParam = topicID
	}

	_, err = database.Exec(
		`INSERT INTO knowledge_open_threads (id, description, blocking_on, topic_id, first_raised_by, source, source_ref, first_raised_at, last_activity_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		db.GenID(), ot.Description, ot.BlockingOn, topicIDParam, ot.FirstRaisedBy,
		sourceType, sourceRef, now, now, status,
	)
	return err
}

// truncate returns at most n characters of s.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// cleanJSON strips markdown code block wrappers from LLM JSON responses
// and extracts the first JSON object or array.
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "[{"); i >= 0 {
		open, close := s[i], byte(']')
		if open == '{' {
			close = '}'
		}
		depth := 0
		for j := i; j < len(s); j++ {
			if s[j] == open {
				depth++
			} else if s[j] == close {
				depth--
				if depth == 0 {
					return s[i : j+1]
				}
			}
		}
	}
	return strings.TrimSpace(s)
}
