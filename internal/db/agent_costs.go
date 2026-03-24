package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Agent cost tracking helpers
// ---------------------------------------------------------------------------

// DBInsertAgentCost inserts a new agent cost row and returns its row ID.
func DBInsertAgentCost(db *sql.DB, agentID, automation string, budget float64, startedAt time.Time) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO cc_agent_costs (agent_id, automation, budget_usd, started_at, status)
		 VALUES (?, ?, ?, ?, 'running')`,
		agentID, automation, budget, FormatTime(startedAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert agent cost: %w", err)
	}
	return res.LastInsertId()
}

// DBUpdateAgentCostFinished marks an agent cost row as finished with final metrics.
func DBUpdateAgentCostFinished(db *sql.DB, rowID int64, durationSec int, costUSD float64, exitCode int, status string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(
		`UPDATE cc_agent_costs
		 SET finished_at = ?, duration_sec = ?, cost_usd = ?, exit_code = ?, status = ?
		 WHERE id = ?`,
		now, durationSec, costUSD, exitCode, status, rowID,
	)
	if err != nil {
		return fmt.Errorf("update agent cost %d: %w", rowID, err)
	}
	return nil
}

// DBSumCostsSince returns the total cost_usd for all agent runs started since the given time.
func DBSumCostsSince(db *sql.DB, since time.Time) (float64, error) {
	var total sql.NullFloat64
	err := db.QueryRow(
		`SELECT SUM(cost_usd) FROM cc_agent_costs WHERE started_at >= ?`,
		FormatTime(since),
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum costs since: %w", err)
	}
	if !total.Valid {
		return 0, nil
	}
	return total.Float64, nil
}

// DBCountLaunchesSince returns the number of agent launches for the given automation since the given time.
func DBCountLaunchesSince(db *sql.DB, automation string, since time.Time) (int, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM cc_agent_costs WHERE automation = ? AND started_at >= ?`,
		automation, FormatTime(since),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count launches since: %w", err)
	}
	return count, nil
}

// DBLastAgentLaunch returns the most recent started_at time for the given agent ID.
// Returns zero time if no launches found.
func DBLastAgentLaunch(db *sql.DB, agentID string) (time.Time, error) {
	var s sql.NullString
	err := db.QueryRow(
		`SELECT MAX(started_at) FROM cc_agent_costs WHERE agent_id = ?`,
		agentID,
	).Scan(&s)
	if err != nil {
		return time.Time{}, fmt.Errorf("last agent launch: %w", err)
	}
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return ParseTime(s.String), nil
}

// DBCountRecentFailures returns the number of failed agent runs for the given automation since the given time.
func DBCountRecentFailures(db *sql.DB, automation string, since time.Time) (int, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM cc_agent_costs WHERE automation = ? AND started_at >= ? AND status = 'failed'`,
		automation, FormatTime(since),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent failures: %w", err)
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Budget state helpers
// ---------------------------------------------------------------------------

// DBGetBudgetState retrieves a budget state value by key.
// Returns (0, "", nil) if the key does not exist.
func DBGetBudgetState(db *sql.DB, key string) (float64, string, error) {
	var num float64
	var text string
	err := db.QueryRow(
		`SELECT value_num, value_text FROM cc_budget_state WHERE key = ?`,
		key,
	).Scan(&num, &text)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("get budget state %q: %w", key, err)
	}
	return num, text, nil
}

// DBSetBudgetState upserts a budget state key-value pair.
func DBSetBudgetState(db *sql.DB, key string, valueNum float64, valueText string) error {
	now := FormatTime(time.Now())
	_, err := db.Exec(
		`INSERT INTO cc_budget_state (key, value_num, value_text, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_num = excluded.value_num, value_text = excluded.value_text, updated_at = excluded.updated_at`,
		key, valueNum, valueText, now,
	)
	if err != nil {
		return fmt.Errorf("set budget state %q: %w", key, err)
	}
	return nil
}
