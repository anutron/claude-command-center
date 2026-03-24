package agent

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
)

// RateLimiter prevents spawn loops and enforces cooldowns. It is stateless in
// memory — all state comes from DB queries, so it survives daemon restarts.
type RateLimiter struct {
	db  *sql.DB
	cfg *config.AgentConfig
}

// NewRateLimiter creates a RateLimiter backed by the given database and config.
func NewRateLimiter(database *sql.DB, cfg *config.AgentConfig) *RateLimiter {
	return &RateLimiter{db: database, cfg: cfg}
}

// CanLaunch checks whether an agent launch is allowed. It returns (true, "")
// if all checks pass, or (false, reason) with a specific denial reason.
//
// Checks are evaluated in order:
//  1. Per-automation hourly cap (skipped if automation is empty)
//  2. Per-agent-ID cooldown
//  3. Failure backoff (skipped if automation is empty)
func (r *RateLimiter) CanLaunch(agentID, automation string) (bool, string) {
	now := time.Now()

	// 1. Per-automation hourly cap
	if automation != "" {
		cap := r.cfg.MaxLaunchesPerAutomationPerHour
		if cap <= 0 {
			cap = 20
		}
		count, err := db.DBCountLaunchesSince(r.db, automation, now.Add(-1*time.Hour))
		if err != nil {
			return false, fmt.Sprintf("rate limit check failed: %v", err)
		}
		if count >= cap {
			return false, fmt.Sprintf("automation %q hit hourly launch cap (%d/%d)", automation, count, cap)
		}
	}

	// 2. Per-agent-ID cooldown
	cooldown := r.cfg.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	lastLaunch, err := db.DBLastAgentLaunch(r.db, agentID)
	if err != nil {
		return false, fmt.Sprintf("cooldown check failed: %v", err)
	}
	if !lastLaunch.IsZero() {
		elapsed := now.Sub(lastLaunch)
		required := time.Duration(cooldown) * time.Minute
		if elapsed < required {
			remaining := required - elapsed
			return false, fmt.Sprintf("agent %q in cooldown (%s remaining)", agentID, remaining.Truncate(time.Second))
		}
	}

	// 3. Failure backoff
	if automation != "" {
		failures, err := db.DBCountRecentFailures(r.db, automation, now.Add(-1*time.Hour))
		if err != nil {
			return false, fmt.Sprintf("failure backoff check failed: %v", err)
		}
		if failures > 0 {
			baseSec := r.cfg.FailureBackoffBaseSec
			if baseSec <= 0 {
				baseSec = 60
			}
			maxSec := r.cfg.FailureBackoffMaxSec
			if maxSec <= 0 {
				maxSec = 3600
			}
			backoffSec := math.Min(float64(baseSec)*math.Pow(2, float64(failures-1)), float64(maxSec))
			backoff := time.Duration(backoffSec) * time.Second

			lastFailure, err := dbLastFailureTime(r.db, automation, now.Add(-1*time.Hour))
			if err != nil {
				return false, fmt.Sprintf("failure backoff check failed: %v", err)
			}
			if !lastFailure.IsZero() && now.Sub(lastFailure) < backoff {
				remaining := backoff - now.Sub(lastFailure)
				return false, fmt.Sprintf("automation %q in failure backoff (%d failures, %s remaining)", automation, failures, remaining.Truncate(time.Second))
			}
		}
	}

	return true, ""
}

// dbLastFailureTime returns the most recent started_at of a failed agent run
// for the given automation since the given time. Returns zero time if none.
func dbLastFailureTime(database *sql.DB, automation string, since time.Time) (time.Time, error) {
	var s sql.NullString
	err := database.QueryRow(
		`SELECT MAX(started_at) FROM cc_agent_costs WHERE automation = ? AND started_at >= ? AND status = 'failed'`,
		automation, db.FormatTime(since),
	).Scan(&s)
	if err != nil {
		return time.Time{}, fmt.Errorf("last failure time: %w", err)
	}
	if !s.Valid || s.String == "" {
		return time.Time{}, nil
	}
	return db.ParseTime(s.String), nil
}
