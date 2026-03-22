package doctor

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// RunDoctor performs all diagnostic checks and prints results.
// It runs built-in checks (config, database, binaries) plus any checks from
// registered DoctorProvider instances.
func RunDoctor(providers []plugin.DoctorProvider, live bool) error {
	opts := plugin.DoctorOpts{Live: live}
	checks := runDoctorChecks(providers, opts)

	okCount := 0
	for _, c := range checks {
		status := "[!!]"
		if c.OK {
			status = "[OK]"
			okCount++
		} else if c.Inconclusive {
			status = "[??]"
		}
		fmt.Printf("  %s %s\n", status, c.Name)
		if !c.OK && c.Message != "" {
			fmt.Printf("       %s\n", c.Message)
		}
	}

	fmt.Printf("\n  %d/%d checks passed\n", okCount, len(checks))
	if okCount < len(checks) {
		return fmt.Errorf("%d checks failed", len(checks)-okCount)
	}
	return nil
}

// DoctorCheck represents a single diagnostic check result.
type DoctorCheck struct {
	Name         string
	OK           bool
	Message      string
	Inconclusive bool
}

func runDoctorChecks(providers []plugin.DoctorProvider, opts plugin.DoctorOpts) []DoctorCheck {
	var checks []DoctorCheck

	// 1. Config file
	checks = append(checks, checkConfig())

	// 2. Database
	checks = append(checks, checkDatabase())

	// 3. Provider checks (calendar, gmail, github, granola, etc.)
	for _, p := range providers {
		for _, pc := range p.DoctorChecks(opts) {
			dc := DoctorCheck{
				Name:         pc.Name,
				Inconclusive: pc.Inconclusive,
			}
			if pc.Result.Status == "ok" {
				dc.OK = true
				dc.Message = pc.Result.Message
			} else {
				dc.OK = false
				dc.Message = pc.Result.Message
				if pc.Result.Hint != "" {
					dc.Message += " — " + pc.Result.Hint
				}
			}
			checks = append(checks, dc)
		}
	}

	// 4. ai-cron binary
	checks = append(checks, checkRefreshBinary())

	// 5. claude CLI
	checks = append(checks, checkClaudeCLI())

	// 6. Data freshness
	checks = append(checks, checkDataFreshness())

	return checks
}

func checkConfig() DoctorCheck {
	_, err := config.Load()
	if err != nil {
		return DoctorCheck{Name: "Config file", OK: false, Message: fmt.Sprintf("Error: %v — run 'ccc setup'", err)}
	}
	return DoctorCheck{Name: "Config file", OK: true}
}

func checkDatabase() DoctorCheck {
	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return DoctorCheck{Name: "Database", OK: false, Message: fmt.Sprintf("Cannot open %s: %v", dbPath, err)}
	}
	database.Close()
	return DoctorCheck{Name: "Database", OK: true}
}

func checkRefreshBinary() DoctorCheck {
	// Check next to current executable
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "ai-cron")
		if _, err := os.Stat(candidate); err == nil {
			return DoctorCheck{Name: "ai-cron binary", OK: true}
		}
	}
	// Check on PATH
	if _, err := exec.LookPath("ai-cron"); err == nil {
		return DoctorCheck{Name: "ai-cron binary", OK: true}
	}
	return DoctorCheck{Name: "ai-cron binary", OK: false, Message: "Not found — run 'make install'"}
}

func checkClaudeCLI() DoctorCheck {
	if _, err := exec.LookPath("claude"); err != nil {
		return DoctorCheck{Name: "claude CLI", OK: false, Message: "Not found on PATH — install from https://claude.ai/claude-code"}
	}
	return DoctorCheck{Name: "claude CLI", OK: true}
}

func checkDataFreshness() DoctorCheck {
	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "Cannot open database"}
	}
	defer database.Close()

	var generatedAt sql.NullString
	err = database.QueryRow("SELECT value FROM cc_meta WHERE key = 'generated_at'").Scan(&generatedAt)
	if err != nil || !generatedAt.Valid {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "No data — run 'ai-cron' to populate"}
	}

	t, err := time.Parse(time.RFC3339, generatedAt.String)
	if err != nil {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "Cannot parse generated_at timestamp"}
	}

	age := time.Since(t)
	if age > 30*time.Minute {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: fmt.Sprintf("Data is %s old — run 'ai-cron'", age.Round(time.Minute))}
	}
	return DoctorCheck{Name: "Data freshness", OK: true, Message: fmt.Sprintf("Data is %s old", age.Round(time.Minute))}
}
