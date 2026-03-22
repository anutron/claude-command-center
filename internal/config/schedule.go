package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const crontabMarker = "# ai-cron schedule"

// IsScheduleInstalled checks if a ai-cron crontab entry exists.
func IsScheduleInstalled() bool {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), crontabMarker)
}

// InstallSchedule adds a crontab entry for ai-cron.
// Uses crontab instead of launchd to avoid macOS "Background Items Added"
// notifications that re-trigger on every binary rebuild.
func InstallSchedule() error {
	cfg, err := Load()
	if err != nil {
		cfg = DefaultConfig()
	}
	interval := cfg.ParseRefreshInterval()

	// Find ai-cron binary
	binary, err := exec.LookPath("ai-cron")
	if err != nil {
		// Try next to current executable
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return fmt.Errorf("ai-cron not found on PATH — run 'make install' first")
		}
		candidate := filepath.Join(filepath.Dir(exe), "ai-cron")
		if _, statErr := os.Stat(candidate); statErr != nil {
			return fmt.Errorf("ai-cron not found on PATH — run 'make install' first")
		}
		binary = candidate
	}

	logPath := filepath.Join(DataDir(), "refresh.log")
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	// Build cron interval (minutes, minimum 1)
	minutes := int(interval.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	cronSchedule := fmt.Sprintf("*/%d * * * *", minutes)

	// Source env file if it exists, then run the binary
	envFile := filepath.Join(ConfigDir(), ".env")
	cronCmd := fmt.Sprintf("[ -f '%s' ] && . '%s'; '%s' >> '%s' 2>&1",
		envFile, envFile, binary, logPath)

	newEntry := fmt.Sprintf("%s %s %s", cronSchedule, cronCmd, crontabMarker)

	// Get existing crontab
	existing := ""
	if out, err := exec.Command("crontab", "-l").Output(); err == nil {
		existing = string(out)
	}

	// Check if already installed with same entry
	if strings.Contains(existing, newEntry) {
		fmt.Println("Schedule already installed (no changes).")
		return nil
	}

	// Remove any old ai-cron entries
	lines := strings.Split(existing, "\n")
	var kept []string
	for _, line := range lines {
		if !strings.Contains(line, crontabMarker) {
			kept = append(kept, line)
		}
	}

	// Add new entry
	kept = append(kept, newEntry)

	// Write back — ensure trailing newline
	newCrontab := strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(newCrontab)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab install failed: %s — %w", string(out), err)
	}

	// Clean up legacy launchd plist if present
	cleanupLegacyPlist()

	fmt.Printf("Installed crontab: %s\n", cronSchedule)
	fmt.Printf("Refresh interval: %s\n", interval)
	fmt.Printf("Log file: %s\n", logPath)
	return nil
}

// UninstallSchedule removes the ai-cron crontab entry.
func UninstallSchedule() error {
	out, _ := exec.Command("crontab", "-l").Output()
	existing := string(out)

	hasCrontab := strings.Contains(existing, crontabMarker)
	if !hasCrontab {
		// Clean up legacy plist even if no crontab entry
		cleanupLegacyPlist()
		fmt.Println("No schedule installed.")
		return nil
	}

	// Remove ai-cron entries
	lines := strings.Split(existing, "\n")
	var kept []string
	for _, line := range lines {
		if !strings.Contains(line, crontabMarker) {
			kept = append(kept, line)
		}
	}

	newCrontab := strings.Join(kept, "\n")
	// If only empty lines remain, clear the crontab entirely
	if strings.TrimSpace(newCrontab) == "" {
		if err := exec.Command("crontab", "-r").Run(); err != nil {
			return fmt.Errorf("crontab remove failed: %w", err)
		}
	} else {
		newCrontab = strings.TrimRight(newCrontab, "\n") + "\n"
		cmd := exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(newCrontab)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("crontab update failed: %s — %w", string(out), err)
		}
	}

	// Also clean up legacy plist
	cleanupLegacyPlist()

	fmt.Println("Schedule uninstalled.")
	return nil
}

// cleanupLegacyPlist removes the old launchd plist if it exists.
func cleanupLegacyPlist() {
	home, _ := os.UserHomeDir()
	plist := filepath.Join(home, "Library", "LaunchAgents", "com.ccc.refresh.plist")

	if _, err := os.Stat(plist); os.IsNotExist(err) {
		return
	}

	// Unload first (ignore errors — may already be unloaded)
	_ = exec.Command("launchctl", "unload", plist).Run()

	if err := os.Remove(plist); err != nil {
		fmt.Printf("Warning: could not remove legacy plist %s: %v\n", plist, err)
	} else {
		fmt.Printf("Cleaned up legacy launchd plist: %s\n", plist)
	}
}
