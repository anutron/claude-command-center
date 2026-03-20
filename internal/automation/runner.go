package automation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// Runner executes configured automations during a refresh cycle.
type Runner struct {
	Automations []config.AutomationConfig
	Config      *config.Config
	DBPath      string
	Logger      plugin.Logger
	Verbose     bool
	LogPath     string // path to the automation log file (for rotation)

	// nowFunc allows tests to inject a fake clock. Defaults to time.Now.
	nowFunc func() time.Time
}

// RunResult captures the outcome of a single automation execution.
type RunResult struct {
	Name    string
	Status  string // "success", "error", "skipped"
	Message string
	Elapsed time.Duration
}

// now returns the current time, using nowFunc if set.
func (r *Runner) now() time.Time {
	if r.nowFunc != nil {
		return r.nowFunc()
	}
	return time.Now()
}

// RunAll executes all enabled and due automations sequentially.
func (r *Runner) RunAll(ctx context.Context, trigger string) []RunResult {
	if len(r.Automations) == 0 {
		return nil
	}

	r.rotateLog()

	db, err := sql.Open("sqlite", r.DBPath)
	if err != nil {
		r.Logger.Error("automation", "failed to open DB", "error", err)
		return nil
	}
	defer db.Close()

	// Ensure the tracking table exists.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS cc_automation_runs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		started_at  TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		status      TEXT NOT NULL,
		message     TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		r.Logger.Error("automation", "failed to create tracking table", "error", err)
		return nil
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_automation_runs_name_started
		ON cc_automation_runs(name, started_at)`); err != nil {
		r.Logger.Error("automation", "failed to create index", "error", err)
	}

	now := r.now()
	var results []RunResult

	for _, auto := range r.Automations {
		if !auto.Enabled {
			results = append(results, RunResult{
				Name:    auto.Name,
				Status:  "skipped",
				Message: "disabled",
			})
			continue
		}

		lastRun := r.getLastRun(db, auto.Name)
		schedule := auto.Schedule
		if schedule == "" {
			schedule = "every_refresh"
		}

		if !isDue(schedule, lastRun, now) {
			results = append(results, RunResult{
				Name:    auto.Name,
				Status:  "skipped",
				Message: "not due",
			})
			continue
		}

		if r.Verbose {
			r.Logger.Info("automation", fmt.Sprintf("running %s (schedule=%s, trigger=%s)", auto.Name, schedule, trigger))
		}

		start := r.now()
		result := r.runOne(ctx, auto, trigger)
		result.Elapsed = r.now().Sub(start)

		r.recordRun(db, auto.Name, start, r.now(), result.Status, result.Message)

		results = append(results, result)
	}

	return results
}

// runOne executes a single automation through the init->run->shutdown handshake.
func (r *Runner) runOne(ctx context.Context, auto config.AutomationConfig, trigger string) RunResult {
	result := RunResult{Name: auto.Name}

	// Create a 30-second timeout context for the entire run.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	proc, err := startProcess(ctx, auto.Command)
	if err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("command not found: %s", auto.Command)
		return result
	}
	defer func() {
		proc.kill()
		proc.wait()
	}()

	// Build scoped config.
	scopedConfig := plugin.ScopeConfig(r.Config, auto.ConfigScopes)
	settings := auto.Settings
	if settings == nil {
		settings = map[string]interface{}{}
	}

	// Step 1: Send init.
	if err := proc.send(HostMsg{
		Type:     "init",
		DBPath:   r.DBPath,
		Config:   scopedConfig,
		Settings: settings,
	}); err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("failed to send init: %v", err)
		return result
	}

	// Step 2: Wait for ready (5s timeout), consuming any log messages
	// that the automation sends during on_init.
	for {
		readyMsg, err := proc.receive(5 * time.Second)
		if err != nil {
			result.Status = "error"
			result.Message = "init timeout"
			return result
		}
		if readyMsg.Type == "log" {
			level := readyMsg.Level
			if level == "" {
				level = "info"
			}
			switch level {
			case "error":
				r.Logger.Error("automation:"+auto.Name, readyMsg.Message)
			case "warn":
				r.Logger.Warn("automation:"+auto.Name, readyMsg.Message)
			default:
				r.Logger.Info("automation:"+auto.Name, readyMsg.Message)
			}
			continue
		}
		if readyMsg.Type != "ready" {
			result.Status = "error"
			result.Message = fmt.Sprintf("expected ready, got %s", readyMsg.Type)
			return result
		}
		break
	}

	// Step 3: Send run.
	if err := proc.send(HostMsg{
		Type:    "run",
		Trigger: trigger,
	}); err != nil {
		result.Status = "error"
		result.Message = fmt.Sprintf("failed to send run: %v", err)
		return result
	}

	// Step 4: Read messages until we get a result.
	for {
		msg, err := proc.receive(30 * time.Second)
		if err != nil {
			result.Status = "error"
			result.Message = fmt.Sprintf("timeout after 30s")
			stderr := proc.stderrOutput(500)
			if stderr != "" {
				result.Message += ": " + stderr
			}
			return result
		}

		switch msg.Type {
		case "log":
			level := msg.Level
			if level == "" {
				level = "info"
			}
			switch level {
			case "error":
				r.Logger.Error("automation:"+auto.Name, msg.Message)
			case "warn":
				r.Logger.Warn("automation:"+auto.Name, msg.Message)
			default:
				r.Logger.Info("automation:"+auto.Name, msg.Message)
			}

		case "result":
			result.Status = msg.Status
			result.Message = msg.Message
			// Send shutdown (best effort).
			_ = proc.send(HostMsg{Type: "shutdown"})
			return result

		default:
			// Ignore unknown message types.
		}
	}
}

// getLastRun returns the most recent run time for the named automation.
func (r *Runner) getLastRun(db *sql.DB, name string) time.Time {
	var startedAt string
	err := db.QueryRow(`SELECT started_at FROM cc_automation_runs
		WHERE name = ? ORDER BY started_at DESC LIMIT 1`, name).Scan(&startedAt)
	if err != nil {
		return time.Time{} // never run
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// recordRun inserts a run record into the tracking table.
func (r *Runner) recordRun(db *sql.DB, name string, started, finished time.Time, status, message string) {
	_, err := db.Exec(`INSERT INTO cc_automation_runs (name, started_at, finished_at, status, message)
		VALUES (?, ?, ?, ?, ?)`,
		name,
		started.UTC().Format(time.RFC3339),
		finished.UTC().Format(time.RFC3339),
		status,
		message,
	)
	if err != nil {
		r.Logger.Error("automation", "failed to record run", "name", name, "error", err)
	}
}

// rotateLog rotates the automation log file if it is older than 7 days or
// exceeds 5MB. Keeps one previous rotation (.1).
func (r *Runner) rotateLog() {
	if r.LogPath == "" {
		return
	}
	info, err := os.Stat(r.LogPath)
	if err != nil {
		return // file doesn't exist yet, nothing to rotate
	}

	sevenDaysAgo := r.now().AddDate(0, 0, -7)
	needsRotation := info.ModTime().Before(sevenDaysAgo) || info.Size() > 5*1024*1024

	if needsRotation {
		rotated := r.LogPath + ".1"
		_ = os.Remove(rotated)
		_ = os.Rename(r.LogPath, rotated)
	}
}

// EnsureTable creates the cc_automation_runs table if it doesn't exist.
// Called during schema migration.
func EnsureTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS cc_automation_runs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		started_at  TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		status      TEXT NOT NULL,
		message     TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_automation_runs_name_started
		ON cc_automation_runs(name, started_at)`)
	return err
}

// LogDir returns the directory where automation logs are stored.
func LogDir() string {
	return filepath.Join(config.DataDir(), "logs")
}
