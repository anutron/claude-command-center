package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ValidateCalendar checks that Google Calendar credentials exist and parse.
func ValidateCalendar() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	credPath := filepath.Join(home, ".config", "google-calendar-mcp", "credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return fmt.Errorf("calendar credentials not found — run 'ccc setup' to configure")
	}

	var creds map[string]interface{}
	if err := json.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("calendar credentials malformed — run 'ccc setup' to reconfigure")
	}

	return nil
}

// ValidateGitHub checks that the GitHub CLI is authenticated.
func ValidateGitHub() error {
	cmd := exec.Command("gh", "auth", "token")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI not authenticated — run 'gh auth login' to authenticate")
	}
	return nil
}

// ValidateGranola checks that Granola stored accounts exist.
func ValidateGranola() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	accountsPath := filepath.Join(home, "Library", "Application Support", "Granola", "stored-accounts.json")
	if _, err := os.Stat(accountsPath); err != nil {
		return fmt.Errorf("Granola not configured — open Granola app to set up")
	}

	return nil
}
