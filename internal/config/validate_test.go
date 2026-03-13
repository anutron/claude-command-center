package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCalendar_MissingCredentials(t *testing.T) {
	// Set HOME to a temp dir so credentials won't be found
	t.Setenv("HOME", t.TempDir())

	err := ValidateCalendar()
	if err == nil {
		t.Error("expected error when calendar credentials don't exist")
	}
}

func TestValidateGitHub_MissingCLI(t *testing.T) {
	// Override PATH so gh won't be found
	t.Setenv("PATH", t.TempDir())

	err := ValidateGitHub()
	if err == nil {
		t.Error("expected error when gh CLI is not available")
	}
}

func TestValidateGranola_MissingAccounts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := ValidateGranola()
	if err == nil {
		t.Error("expected error when Granola accounts don't exist")
	}
}

func TestValidateGmail_MissingCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := ValidateGmail()
	if err == nil {
		t.Error("expected error when Gmail credentials don't exist")
	}
}

func TestValidateGmail_ValidCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create the expected credentials file
	gmailDir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(gmailDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tokenContent := `{"access_token": "test", "refresh_token": "test"}`
	if err := os.WriteFile(filepath.Join(gmailDir, "work.json"), []byte(tokenContent), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateGmail()
	if err != nil {
		t.Errorf("expected no error with valid credentials, got: %v", err)
	}
}

func TestValidateGmail_MalformedCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	gmailDir := filepath.Join(tmpHome, ".gmail-mcp")
	if err := os.MkdirAll(gmailDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gmailDir, "work.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateGmail()
	if err == nil {
		t.Error("expected error for malformed Gmail credentials")
	}
}

func TestValidateSlack_MissingToken(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "")
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())

	err := ValidateSlack()
	if err == nil {
		t.Error("expected error when SLACK_BOT_TOKEN is not set")
	}
}

func TestIsScheduleInstalled_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if IsScheduleInstalled() {
		t.Error("expected false when plist does not exist")
	}
}
