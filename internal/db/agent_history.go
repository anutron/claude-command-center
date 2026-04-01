package db

import (
	"database/sql"
	"fmt"
	"time"
)

// AgentHistoryEntry is a joined view of an agent run with its origin context.
type AgentHistoryEntry struct {
	// From cc_agent_costs
	AgentID      string     `json:"agent_id"`
	Automation   string     `json:"automation"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	DurationSec  int        `json:"duration_sec"`
	CostUSD      float64    `json:"cost_usd"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	ExitCode     *int       `json:"exit_code,omitempty"`

	// Origin context (from joins)
	OriginType  string `json:"origin_type"`  // "todo", "pr", "manual"
	OriginLabel string `json:"origin_label"` // e.g. "TODO #113 — Fix auth bug"
	OriginRef   string `json:"origin_ref"`   // e.g. "todo:113" or "pr:47:review"

	// From cc_sessions (if linked)
	ProjectDir string `json:"project_dir,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Branch     string `json:"branch,omitempty"`
	SessionID  string `json:"session_id,omitempty"` // Claude session UUID
}

// DBLoadAgentHistory returns agent runs from the last `window` duration,
// joined with todo/PR origin data. Ordered by started_at DESC.
func DBLoadAgentHistory(database *sql.DB, window time.Duration) ([]AgentHistoryEntry, error) {
	since := FormatTime(time.Now().Add(-window))
	rows, err := database.Query(`
		SELECT
			ac.agent_id,
			ac.automation,
			ac.status,
			ac.started_at,
			ac.finished_at,
			ac.duration_sec,
			ac.cost_usd,
			ac.input_tokens,
			ac.output_tokens,
			ac.exit_code,
			t.display_id,
			t.title,
			pr.number,
			pr.title,
			pr.agent_category,
			s.session_id,
			s.project,
			s.repo,
			s.branch
		FROM cc_agent_costs ac
		LEFT JOIN cc_todos t ON t.session_id = ac.agent_id
		LEFT JOIN cc_pull_requests pr ON pr.agent_session_id = ac.agent_id
		LEFT JOIN cc_sessions s ON s.session_id = ac.agent_id
		WHERE ac.started_at >= ?
		ORDER BY ac.started_at DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("load agent history: %w", err)
	}
	defer rows.Close()

	var entries []AgentHistoryEntry
	for rows.Next() {
		var e AgentHistoryEntry
		var finishedAt, sessionID, project, repo, branch sql.NullString
		var exitCode, durationSec sql.NullInt64
		var inputTokens, outputTokens sql.NullInt64
		var costUSD sql.NullFloat64
		var todoDisplayID sql.NullInt64
		var todoTitle, prTitle, prCategory sql.NullString
		var prNumber sql.NullInt64
		var startedAtStr string

		err := rows.Scan(
			&e.AgentID, &e.Automation, &e.Status, &startedAtStr,
			&finishedAt, &durationSec, &costUSD,
			&inputTokens, &outputTokens, &exitCode,
			&todoDisplayID, &todoTitle,
			&prNumber, &prTitle, &prCategory,
			&sessionID, &project, &repo, &branch,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agent history row: %w", err)
		}

		e.StartedAt = ParseTime(startedAtStr)
		if finishedAt.Valid {
			t := ParseTime(finishedAt.String)
			e.FinishedAt = &t
		}
		if durationSec.Valid {
			e.DurationSec = int(durationSec.Int64)
		}
		if costUSD.Valid {
			e.CostUSD = costUSD.Float64
		}
		if inputTokens.Valid {
			e.InputTokens = int(inputTokens.Int64)
		}
		if outputTokens.Valid {
			e.OutputTokens = int(outputTokens.Int64)
		}
		if exitCode.Valid {
			ec := int(exitCode.Int64)
			e.ExitCode = &ec
		}
		if sessionID.Valid {
			e.SessionID = sessionID.String
		}
		if project.Valid {
			e.ProjectDir = project.String
		}
		if repo.Valid {
			e.Repo = repo.String
		}
		if branch.Valid {
			e.Branch = branch.String
		}

		switch {
		case todoDisplayID.Valid && todoTitle.Valid:
			e.OriginType = "todo"
			e.OriginLabel = fmt.Sprintf("TODO #%d — %s", todoDisplayID.Int64, todoTitle.String)
			e.OriginRef = fmt.Sprintf("todo:%d", todoDisplayID.Int64)
		case prNumber.Valid && prTitle.Valid:
			e.OriginType = "pr"
			cat := "review"
			if prCategory.Valid {
				cat = prCategory.String
			}
			e.OriginLabel = fmt.Sprintf("PR #%d — %s", prNumber.Int64, prTitle.String)
			e.OriginRef = fmt.Sprintf("pr:%d:%s", prNumber.Int64, cat)
		default:
			e.OriginType = "manual"
			e.OriginLabel = e.AgentID
			e.OriginRef = "manual"
		}

		entries = append(entries, e)
	}
	return entries, rows.Err()
}
