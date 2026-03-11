package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const plistLabel = "com.ccc.refresh"

var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.Binary}}</string>
    </array>
    <key>StartInterval</key>
    <integer>{{.IntervalSeconds}}</integer>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`))

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

// IsScheduleInstalled checks if the launchd plist file exists.
func IsScheduleInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

// InstallSchedule generates a launchd plist for ccc-refresh and loads it.
func InstallSchedule() error {
	cfg, err := Load()
	if err != nil {
		cfg = DefaultConfig()
	}
	interval := cfg.ParseRefreshInterval()

	// Find ccc-refresh binary
	binary, err := exec.LookPath("ccc-refresh")
	if err != nil {
		// Try next to current executable
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return fmt.Errorf("ccc-refresh not found on PATH — run 'make install' first")
		}
		candidate := filepath.Join(filepath.Dir(exe), "ccc-refresh")
		if _, statErr := os.Stat(candidate); statErr != nil {
			return fmt.Errorf("ccc-refresh not found on PATH — run 'make install' first")
		}
		binary = candidate
	}

	logPath := filepath.Join(DataDir(), "refresh.log")
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	// Ensure LaunchAgents directory exists
	dir := filepath.Dir(plistPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}

	// Write plist
	f, err := os.Create(plistPath())
	if err != nil {
		return fmt.Errorf("creating plist: %w", err)
	}
	defer f.Close()

	data := struct {
		Label           string
		Binary          string
		IntervalSeconds int
		LogPath         string
	}{
		Label:           plistLabel,
		Binary:          binary,
		IntervalSeconds: int(interval.Seconds()),
		LogPath:         logPath,
	}
	if err := plistTemplate.Execute(f, data); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	// Load with launchctl
	cmd := exec.Command("launchctl", "load", plistPath())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %s — %w", string(out), err)
	}

	fmt.Printf("Installed: %s\n", plistPath())
	fmt.Printf("Refresh interval: %s\n", interval)
	fmt.Printf("Log file: %s\n", logPath)
	return nil
}

// UninstallSchedule unloads and removes the launchd plist.
func UninstallSchedule() error {
	path := plistPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("No schedule installed.")
		return nil
	}

	cmd := exec.Command("launchctl", "unload", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Warning: launchctl unload: %s — %v\n", string(out), err)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing plist: %w", err)
	}

	fmt.Printf("Uninstalled: %s\n", path)
	return nil
}
