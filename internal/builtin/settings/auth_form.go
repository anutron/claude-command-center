package settings

import (
	"errors"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
)

// Form descriptions. Each points the user at the environment-variable path
// (e.g. sourced from 1Password via `secret <VAR>`) so secrets need not be typed
// into — and saved into — plaintext config.
const (
	slackTokenDescription = "Optional if SLACK_TOKEN is set in your environment " +
		"(e.g. `secret SLACK_TOKEN`) — otherwise paste your xoxp- user token from " +
		"api.slack.com/apps > OAuth & Permissions."
	clientIDDescription = "Optional if GOOGLE_CLIENT_ID is set in your environment " +
		"(e.g. `secret GOOGLE_CLIENT_ID`) — otherwise from Google Cloud Console > " +
		"APIs & Services > Credentials."
	clientSecretDescription = "Optional if GOOGLE_CLIENT_SECRET is set in your environment " +
		"— otherwise the client secret for your OAuth 2.0 credentials."
)

// envSet reports whether any of the named environment variables is set to a
// non-empty (trimmed) value.
func envSet(names ...string) bool {
	for _, n := range names {
		if strings.TrimSpace(os.Getenv(n)) != "" {
			return true
		}
	}
	return false
}

// validateSlackToken allows an empty value when the token is already supplied
// via the environment ($SLACK_TOKEN or the deprecated $SLACK_BOT_TOKEN);
// otherwise a non-empty value is required.
func validateSlackToken(s string) error {
	if strings.TrimSpace(s) == "" && !envSet("SLACK_TOKEN", "SLACK_BOT_TOKEN") {
		return errors.New("required (or set $SLACK_TOKEN)")
	}
	return nil
}

// validateGoogleClientID allows an empty value when $GOOGLE_CLIENT_ID is set.
func validateGoogleClientID(s string) error {
	if strings.TrimSpace(s) == "" && !envSet("GOOGLE_CLIENT_ID") {
		return errors.New("required (or set $GOOGLE_CLIENT_ID)")
	}
	return nil
}

// validateGoogleClientSecret allows an empty value when $GOOGLE_CLIENT_SECRET is set.
func validateGoogleClientSecret(s string) error {
	if strings.TrimSpace(s) == "" && !envSet("GOOGLE_CLIENT_SECRET") {
		return errors.New("required (or set $GOOGLE_CLIENT_SECRET)")
	}
	return nil
}

// clientCredentials holds the values collected from the credential form.
type clientCredentials struct {
	ClientID     string
	ClientSecret string
}

// slackTokenValue holds the value collected from the Slack token form.
type slackTokenValue struct {
	Token string
}

// newClientCredForm creates a huh form for entering Google OAuth client credentials.
// The form collects a Client ID and Client Secret. Both may be left empty when
// the matching env var (GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET) is set, in which
// case the OAuth flow falls back to the environment.
func newClientCredForm(theme *huh.Theme) (*huh.Form, *clientCredentials) {
	creds := &clientCredentials{}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Client ID").
				Description(clientIDDescription).
				Value(&creds.ClientID).
				Validate(validateGoogleClientID),
			huh.NewInput().
				Title("Client Secret").
				Description(clientSecretDescription).
				Value(&creds.ClientSecret).
				EchoMode(huh.EchoModePassword).
				Validate(validateGoogleClientSecret),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(theme)

	return form, creds
}

// newSlackTokenForm creates a huh form for entering a Slack user token. The
// value may be left empty when $SLACK_TOKEN (or the deprecated $SLACK_BOT_TOKEN)
// is set in the environment.
func newSlackTokenForm(theme *huh.Theme) (*huh.Form, *slackTokenValue) {
	tok := &slackTokenValue{}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Slack User Token").
				Description(slackTokenDescription).
				Value(&tok.Token).
				EchoMode(huh.EchoModePassword).
				Validate(validateSlackToken),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(theme)

	return form, tok
}
