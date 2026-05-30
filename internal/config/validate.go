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

// ResolveSlackToken returns the Slack user token for the given (already loaded)
// config, falling back to the environment. It is the single source of token
// precedence used by both credential validation and every data-fetch path:
//
//	config Token > config BotToken (deprecated) > $SLACK_TOKEN > $SLACK_BOT_TOKEN (deprecated)
//
// A nil config is tolerated and falls straight through to the environment. The
// environment fallback is what lets the token live exclusively in $SLACK_TOKEN
// (e.g. exported from 1Password) with no value in config.yaml — fetch paths must
// use this rather than cfg.Slack.EffectiveToken(), which reads config only.
func ResolveSlackToken(cfg *Config) string {
	if cfg != nil {
		if tok := strings.TrimSpace(cfg.Slack.EffectiveToken()); tok != "" {
			return tok
		}
	}
	if tok := strings.TrimSpace(os.Getenv("SLACK_TOKEN")); tok != "" {
		return tok
	}
	return strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN"))
}

// LoadSlackToken loads the config from disk and resolves the Slack token via
// ResolveSlackToken. Used by credential validation and the settings UI.
func LoadSlackToken() string {
	cfg, err := Load()
	if err != nil {
		cfg = nil
	}
	return ResolveSlackToken(cfg)
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
