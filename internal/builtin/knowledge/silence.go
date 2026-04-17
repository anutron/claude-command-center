package knowledge

import (
	"database/sql"
	"fmt"
	"log"
	"time"

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
// For each qualifying item, it upserts a row to knowledge_surfaced_insights.
// It also removes stale insights whose underlying condition no longer holds.
// Publishes knowledge.insights.updated on the bus when changes occur.
func RunSilenceAnalysis(database *sql.DB, bus plugin.EventBus, cfg SilenceConfig) error {
	changed := false

	// --- Collect qualifying topics ---
	type silentTopic struct {
		id, name, lastSeen string
		mentionCount       int
	}

	topicThreshold := time.Now().Add(-time.Duration(cfg.TopicSilenceDays) * 24 * time.Hour).Format(time.RFC3339)
	topicRows, err := database.Query(
		`SELECT id, name, mention_count, last_seen FROM knowledge_topics
		 WHERE last_seen < ? AND mention_count > 3`,
		topicThreshold,
	)
	if err != nil {
		return fmt.Errorf("querying silent topics: %w", err)
	}

	var silentTopics []silentTopic
	for topicRows.Next() {
		var st silentTopic
		if err := topicRows.Scan(&st.id, &st.name, &st.mentionCount, &st.lastSeen); err != nil {
			log.Printf("silence: scanning topic row: %v", err)
			continue
		}
		silentTopics = append(silentTopics, st)
	}
	topicRows.Close()

	// --- Write topic silence alerts ---
	qualifyingTopicIDs := map[string]bool{}
	for _, st := range silentTopics {
		qualifyingTopicIDs[st.id] = true

		insightID := fmt.Sprintf("silence-topic-%s", st.id)
		title := fmt.Sprintf("No activity on: %s", st.name)

		lastSeenTime, _ := time.Parse(time.RFC3339, st.lastSeen)
		daysAgo := int(time.Since(lastSeenTime).Hours() / 24)
		body := fmt.Sprintf("Last mentioned %d days ago, discussed %d times", daysAgo, st.mentionCount)

		if err := upsertSilenceInsight(database, insightID, title, body); err != nil {
			log.Printf("silence: upserting topic insight %s: %v", insightID, err)
			continue
		}
		changed = true
	}

	// --- Collect qualifying threads ---
	type silentThread struct {
		id, description, lastActivity string
	}

	threadThreshold := time.Now().Add(-time.Duration(cfg.ThreadSilenceDays) * 24 * time.Hour).Format(time.RFC3339)
	threadRows, err := database.Query(
		`SELECT id, description, last_activity_at FROM knowledge_open_threads
		 WHERE last_activity_at < ? AND first_raised_by = 'Aaron' AND status = 'open'`,
		threadThreshold,
	)
	if err != nil {
		return fmt.Errorf("querying silent threads: %w", err)
	}

	var silentThreads []silentThread
	for threadRows.Next() {
		var st silentThread
		if err := threadRows.Scan(&st.id, &st.description, &st.lastActivity); err != nil {
			log.Printf("silence: scanning thread row: %v", err)
			continue
		}
		silentThreads = append(silentThreads, st)
	}
	threadRows.Close()

	// --- Write thread silence alerts ---
	qualifyingThreadIDs := map[string]bool{}
	for _, st := range silentThreads {
		qualifyingThreadIDs[st.id] = true

		insightID := fmt.Sprintf("silence-thread-%s", st.id)
		title := fmt.Sprintf("Open thread gone quiet: %s", st.description)

		lastActivityTime, _ := time.Parse(time.RFC3339, st.lastActivity)
		daysAgo := int(time.Since(lastActivityTime).Hours() / 24)
		body := fmt.Sprintf("No activity for %d days, raised by Aaron", daysAgo)

		if err := upsertSilenceInsight(database, insightID, title, body); err != nil {
			log.Printf("silence: upserting thread insight %s: %v", insightID, err)
			continue
		}
		changed = true
	}

	// --- Remove stale insights ---
	staleRemoved, err := removeStaleInsights(database, qualifyingTopicIDs, qualifyingThreadIDs)
	if err != nil {
		log.Printf("silence: removing stale insights: %v", err)
	}
	if staleRemoved {
		changed = true
	}

	// Publish event if any changes were made
	if changed && bus != nil {
		bus.Publish(plugin.Event{
			Source: "knowledge",
			Topic:  "knowledge.insights.updated",
		})
	}

	return nil
}

// upsertSilenceInsight inserts a new silence insight or updates an existing one.
// If the insight was previously dismissed, it is left unchanged.
func upsertSilenceInsight(database *sql.DB, id, title, body string) error {
	// Check if it already exists
	var existingID string
	var dismissedAt *string
	err := database.QueryRow(
		"SELECT id, dismissed_at FROM knowledge_surfaced_insights WHERE id = ?", id,
	).Scan(&existingID, &dismissedAt)

	if err == sql.ErrNoRows {
		// Insert new insight
		_, err = database.Exec(
			`INSERT INTO knowledge_surfaced_insights (id, type, title, body, priority, surfaced_at)
			 VALUES (?, 'silence_alert', ?, ?, 50, datetime('now'))`,
			id, title, body,
		)
		return err
	}
	if err != nil {
		return err
	}

	// Existing insight: if dismissed, leave it alone
	if dismissedAt != nil {
		return nil
	}

	// Update the existing non-dismissed insight
	_, err = database.Exec(
		`UPDATE knowledge_surfaced_insights SET title = ?, body = ?, surfaced_at = datetime('now') WHERE id = ?`,
		title, body, id,
	)
	return err
}

// removeStaleInsights deletes silence alert insights whose underlying condition
// no longer qualifies (topic was mentioned again, thread resolved, etc.).
func removeStaleInsights(database *sql.DB, qualifyingTopicIDs, qualifyingThreadIDs map[string]bool) (bool, error) {
	// Find all existing silence_alert insights
	rows, err := database.Query(
		`SELECT id FROM knowledge_surfaced_insights WHERE type = 'silence_alert'`,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var toDelete []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}

		// Parse the insight ID to determine what it refers to
		var artifactID string
		isTopicInsight := false
		isThreadInsight := false

		if n, _ := fmt.Sscanf(id, "silence-topic-%s", &artifactID); n == 1 {
			isTopicInsight = true
			// Sscanf stops at whitespace, so for IDs with dashes this works fine.
			// But we need the full suffix after the prefix.
			artifactID = id[len("silence-topic-"):]
		} else if n, _ := fmt.Sscanf(id, "silence-thread-%s", &artifactID); n == 1 {
			isThreadInsight = true
			artifactID = id[len("silence-thread-"):]
		}

		if isTopicInsight && !qualifyingTopicIDs[artifactID] {
			toDelete = append(toDelete, id)
		}
		if isThreadInsight && !qualifyingThreadIDs[artifactID] {
			toDelete = append(toDelete, id)
		}
	}

	removed := false
	for _, id := range toDelete {
		_, err := database.Exec("DELETE FROM knowledge_surfaced_insights WHERE id = ?", id)
		if err != nil {
			log.Printf("silence: deleting stale insight %s: %v", id, err)
			continue
		}
		removed = true
	}

	return removed, nil
}
