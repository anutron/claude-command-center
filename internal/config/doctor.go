package config

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
)

// DoctorCheck represents a single diagnostic check result.
type DoctorCheck struct {
	Name    string
	OK      bool
	Message string
}

// RunDoctor performs all diagnostic checks and prints results.
func RunDoctor() error {
	checks := runDoctorChecks()

	okCount := 0
	for _, c := range checks {
		status := "[!!]"
		if c.OK {
			status = "[OK]"
			okCount++
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

func runDoctorChecks() []DoctorCheck {
	var checks []DoctorCheck

	// 1. Config file
	checks = append(checks, checkConfig())

	// 2. Database
	checks = append(checks, checkDatabase())

	// 3. Calendar credentials
	checks = append(checks, checkCalendar())

	// 4. GitHub CLI
	checks = append(checks, checkGitHub())

	// 5. Granola
	checks = append(checks, checkGranola())

	// 6. ccc-refresh binary
	checks = append(checks, checkRefreshBinary())

	// 7. claude CLI
	checks = append(checks, checkClaudeCLI())

	// 8. Data freshness
	checks = append(checks, checkDataFreshness())

	return checks
}

func checkConfig() DoctorCheck {
	_, err := Load()
	if err != nil {
		return DoctorCheck{Name: "Config file", OK: false, Message: fmt.Sprintf("Error: %v — run 'ccc setup'", err)}
	}
	return DoctorCheck{Name: "Config file", OK: true}
}

func checkDatabase() DoctorCheck {
	dbPath := DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return DoctorCheck{Name: "Database", OK: false, Message: fmt.Sprintf("Cannot open %s: %v", dbPath, err)}
	}
	database.Close()
	return DoctorCheck{Name: "Database", OK: true}
}

func checkCalendar() DoctorCheck {
	if err := ValidateCalendar(); err != nil {
		return DoctorCheck{Name: "Calendar credentials", OK: false, Message: err.Error()}
	}
	return DoctorCheck{Name: "Calendar credentials", OK: true}
}

func checkGitHub() DoctorCheck {
	if err := ValidateGitHub(); err != nil {
		return DoctorCheck{Name: "GitHub CLI", OK: false, Message: err.Error()}
	}
	return DoctorCheck{Name: "GitHub CLI", OK: true}
}

func checkGranola() DoctorCheck {
	if err := ValidateGranola(); err != nil {
		return DoctorCheck{Name: "Granola", OK: false, Message: err.Error()}
	}
	return DoctorCheck{Name: "Granola", OK: true}
}

func checkRefreshBinary() DoctorCheck {
	// Check next to current executable
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "ccc-refresh")
		if _, err := os.Stat(candidate); err == nil {
			return DoctorCheck{Name: "ccc-refresh binary", OK: true}
		}
	}
	// Check on PATH
	if _, err := exec.LookPath("ccc-refresh"); err == nil {
		return DoctorCheck{Name: "ccc-refresh binary", OK: true}
	}
	return DoctorCheck{Name: "ccc-refresh binary", OK: false, Message: "Not found — run 'make install'"}
}

func checkClaudeCLI() DoctorCheck {
	if _, err := exec.LookPath("claude"); err != nil {
		return DoctorCheck{Name: "claude CLI", OK: false, Message: "Not found on PATH — install from https://claude.ai/claude-code"}
	}
	return DoctorCheck{Name: "claude CLI", OK: true}
}

func checkDataFreshness() DoctorCheck {
	dbPath := DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "Cannot open database"}
	}
	defer database.Close()

	var generatedAt sql.NullString
	err = database.QueryRow("SELECT value FROM cc_meta WHERE key = 'generated_at'").Scan(&generatedAt)
	if err != nil || !generatedAt.Valid {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "No data — run 'ccc-refresh' to populate"}
	}

	t, err := time.Parse(time.RFC3339, generatedAt.String)
	if err != nil {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: "Cannot parse generated_at timestamp"}
	}

	age := time.Since(t)
	if age > 30*time.Minute {
		return DoctorCheck{Name: "Data freshness", OK: false, Message: fmt.Sprintf("Data is %s old — run 'ccc-refresh'", age.Round(time.Minute))}
	}
	return DoctorCheck{Name: "Data freshness", OK: true, Message: fmt.Sprintf("Data is %s old", age.Round(time.Minute))}
}
