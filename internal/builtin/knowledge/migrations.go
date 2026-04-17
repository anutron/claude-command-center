package knowledge

import "github.com/anutron/claude-command-center/internal/plugin"

// knowledgeMigrations returns the ordered migrations for the knowledge plugin.
// All tables are namespaced with the "knowledge_" prefix.
func knowledgeMigrations() []plugin.Migration {
	return []plugin.Migration{
		{
			Version: 1,
			SQL: `
				CREATE TABLE IF NOT EXISTS knowledge_topics (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL UNIQUE COLLATE NOCASE,
					description TEXT,
					first_seen TEXT NOT NULL,
					last_seen TEXT NOT NULL,
					mention_count INTEGER NOT NULL DEFAULT 0
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_topics_name
					ON knowledge_topics(name);

				CREATE TABLE IF NOT EXISTS knowledge_decisions (
					id TEXT PRIMARY KEY,
					title TEXT NOT NULL,
					description TEXT NOT NULL,
					alternatives TEXT,
					reasoning TEXT,
					participants TEXT,
					aaron_present INTEGER NOT NULL,
					source TEXT NOT NULL,
					source_ref TEXT NOT NULL,
					decided_at TEXT NOT NULL,
					extracted_at TEXT NOT NULL
				);

				CREATE TABLE IF NOT EXISTS knowledge_positions (
					id TEXT PRIMARY KEY,
					holder TEXT NOT NULL,
					position TEXT NOT NULL,
					topic_id TEXT,
					source TEXT NOT NULL,
					source_ref TEXT NOT NULL,
					stated_at TEXT NOT NULL,
					extracted_at TEXT NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_knowledge_positions_holder_topic
					ON knowledge_positions(holder, topic_id);

				CREATE TABLE IF NOT EXISTS knowledge_open_threads (
					id TEXT PRIMARY KEY,
					description TEXT NOT NULL,
					blocking_on TEXT,
					topic_id TEXT,
					first_raised_by TEXT,
					source TEXT NOT NULL,
					source_ref TEXT NOT NULL,
					first_raised_at TEXT NOT NULL,
					last_activity_at TEXT NOT NULL,
					status TEXT NOT NULL,
					resolved_by TEXT
				);
				CREATE INDEX IF NOT EXISTS idx_knowledge_open_threads_last_activity
					ON knowledge_open_threads(last_activity_at);
				CREATE INDEX IF NOT EXISTS idx_knowledge_open_threads_raised_by
					ON knowledge_open_threads(first_raised_by, last_activity_at);

				CREATE TABLE IF NOT EXISTS knowledge_edges (
					from_id TEXT NOT NULL,
					from_type TEXT NOT NULL,
					to_id TEXT NOT NULL,
					to_type TEXT NOT NULL,
					relationship TEXT NOT NULL,
					created_at TEXT NOT NULL,
					PRIMARY KEY (from_id, to_id, relationship)
				);

				CREATE TABLE IF NOT EXISTS knowledge_surfaced_insights (
					id TEXT PRIMARY KEY,
					type TEXT NOT NULL,
					title TEXT NOT NULL,
					body TEXT NOT NULL,
					source_refs TEXT,
					priority INTEGER NOT NULL DEFAULT 50,
					surfaced_at TEXT NOT NULL,
					dismissed_at TEXT
				);
				CREATE INDEX IF NOT EXISTS idx_knowledge_surfaced_insights_dismissed
					ON knowledge_surfaced_insights(dismissed_at);

				CREATE TABLE IF NOT EXISTS knowledge_backfill_state (
					source TEXT PRIMARY KEY,
					last_offset TEXT NOT NULL DEFAULT '',
					completed INTEGER NOT NULL DEFAULT 0,
					updated_at TEXT NOT NULL
				);
				INSERT OR IGNORE INTO knowledge_backfill_state (source, last_offset, completed, updated_at)
					VALUES ('granola', '', 0, datetime('now'));
				INSERT OR IGNORE INTO knowledge_backfill_state (source, last_offset, completed, updated_at)
					VALUES ('slack', '', 0, datetime('now'));
				INSERT OR IGNORE INTO knowledge_backfill_state (source, last_offset, completed, updated_at)
					VALUES ('gmail', '', 0, datetime('now'));
			`,
		},
	}
}
