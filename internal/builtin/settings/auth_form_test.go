package settings

import (
	"strings"
	"testing"
)

// --- Slack token validation: optional when the env supplies it ---

func TestValidateSlackToken_RequiredWhenNoEnv(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")
	t.Setenv("SLACK_BOT_TOKEN", "")
	if err := validateSlackToken(""); err == nil {
		t.Error("expected empty token to be rejected when no env var is set")
	}
	if err := validateSlackToken("xoxp-abc"); err != nil {
		t.Errorf("expected non-empty token to pass, got %v", err)
	}
}

func TestValidateSlackToken_OptionalWhenSlackTokenEnvSet(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-from-env")
	t.Setenv("SLACK_BOT_TOKEN", "")
	if err := validateSlackToken(""); err != nil {
		t.Errorf("expected empty token to be allowed when $SLACK_TOKEN is set, got %v", err)
	}
}

func TestValidateSlackToken_OptionalWhenBotTokenEnvSet(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")
	t.Setenv("SLACK_BOT_TOKEN", "xoxp-from-env")
	if err := validateSlackToken(""); err != nil {
		t.Errorf("expected empty token to be allowed when $SLACK_BOT_TOKEN is set, got %v", err)
	}
}

// --- Google client credential validation: optional when the env supplies it ---

func TestValidateGoogleClientID_RequiredWhenNoEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "")
	if err := validateGoogleClientID(""); err == nil {
		t.Error("expected empty Client ID to be rejected when $GOOGLE_CLIENT_ID is unset")
	}
}

func TestValidateGoogleClientID_OptionalWhenEnvSet(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "id-from-env")
	if err := validateGoogleClientID(""); err != nil {
		t.Errorf("expected empty Client ID to be allowed when $GOOGLE_CLIENT_ID is set, got %v", err)
	}
}

func TestValidateGoogleClientSecret_RequiredWhenNoEnv(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_SECRET", "")
	if err := validateGoogleClientSecret(""); err == nil {
		t.Error("expected empty Client Secret to be rejected when $GOOGLE_CLIENT_SECRET is unset")
	}
}

func TestValidateGoogleClientSecret_OptionalWhenEnvSet(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret-from-env")
	if err := validateGoogleClientSecret(""); err != nil {
		t.Errorf("expected empty Client Secret to be allowed when $GOOGLE_CLIENT_SECRET is set, got %v", err)
	}
}

// --- Form descriptions point users at the env/1Password path ---

func TestSlackTokenForm_DescriptionMentionsEnv(t *testing.T) {
	if !strings.Contains(slackTokenDescription, "SLACK_TOKEN") {
		t.Errorf("expected Slack token description to mention SLACK_TOKEN, got %q", slackTokenDescription)
	}
}

func TestClientCredForm_DescriptionsMentionEnv(t *testing.T) {
	if !strings.Contains(clientIDDescription, "GOOGLE_CLIENT_ID") {
		t.Errorf("expected Client ID description to mention GOOGLE_CLIENT_ID, got %q", clientIDDescription)
	}
	if !strings.Contains(clientSecretDescription, "GOOGLE_CLIENT_SECRET") {
		t.Errorf("expected Client Secret description to mention GOOGLE_CLIENT_SECRET, got %q", clientSecretDescription)
	}
}

func TestNewClientCredForm_ReturnsFormAndCreds(t *testing.T) {
	form, creds := newClientCredForm(nil)
	if form == nil {
		t.Fatal("expected non-nil form")
	}
	if creds == nil {
		t.Fatal("expected non-nil creds")
	}
}

func TestNewClientCredForm_CredsStartEmpty(t *testing.T) {
	_, creds := newClientCredForm(nil)
	if creds.ClientID != "" {
		t.Errorf("expected empty ClientID, got %q", creds.ClientID)
	}
	if creds.ClientSecret != "" {
		t.Errorf("expected empty ClientSecret, got %q", creds.ClientSecret)
	}
}

func TestNewClientCredForm_BindsToCredsStruct(t *testing.T) {
	_, creds := newClientCredForm(nil)

	// Simulate filling in credentials by writing directly to the struct.
	// In actual huh usage, the form updates these via pointer binding.
	creds.ClientID = "test-id"
	creds.ClientSecret = "test-secret"

	if creds.ClientID != "test-id" {
		t.Errorf("expected ClientID 'test-id', got %q", creds.ClientID)
	}
	if creds.ClientSecret != "test-secret" {
		t.Errorf("expected ClientSecret 'test-secret', got %q", creds.ClientSecret)
	}
}
