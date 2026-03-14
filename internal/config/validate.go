package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ValidateSlack checks that a Slack token is available.
// It checks the config file first, then falls back to env vars.
func ValidateSlack() error {
	if LoadSlackToken() == "" {
		return fmt.Errorf("Slack token not configured — press 'a' to enter token or export SLACK_TOKEN")
	}
	return nil
}

// LoadSlackToken returns the Slack user token from config or environment.
// Config Token field takes precedence over the deprecated BotToken field,
// which takes precedence over environment variables.
func LoadSlackToken() string {
	// Check config file first — prefer Token over deprecated BotToken
	cfg, err := Load()
	if err == nil {
		if cfg.Slack.Token != "" {
			return strings.TrimSpace(cfg.Slack.Token)
		}
		if cfg.Slack.BotToken != "" {
			return strings.TrimSpace(cfg.Slack.BotToken)
		}
	}
	// Fall back to environment variables — prefer SLACK_TOKEN over deprecated SLACK_BOT_TOKEN
	if tok := strings.TrimSpace(os.Getenv("SLACK_TOKEN")); tok != "" {
		return tok
	}
	return strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN"))
}

// ValidateGmail checks that Gmail MCP server credentials exist.
func ValidateGmail() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	tokenPath := filepath.Join(home, ".gmail-mcp", "work.json")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("gmail credentials not found at %s — run gmail MCP auth to configure", tokenPath)
	}

	var token map[string]interface{}
	if err := json.Unmarshal(data, &token); err != nil {
		return fmt.Errorf("gmail credentials malformed — re-run gmail MCP auth to reconfigure")
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
