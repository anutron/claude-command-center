package settings

import (
	"errors"

	"github.com/charmbracelet/huh"
)

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
// The form collects a Client ID and Client Secret, both required non-empty.
func newClientCredForm(theme *huh.Theme) (*huh.Form, *clientCredentials) {
	creds := &clientCredentials{}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Client ID").
				Description("From Google Cloud Console > APIs & Services > Credentials").
				Value(&creds.ClientID).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Client Secret").
				Description("The client secret for your OAuth 2.0 credentials").
				Value(&creds.ClientSecret).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(theme)

	return form, creds
}

// newSlackTokenForm creates a huh form for entering a Slack user token.
func newSlackTokenForm(theme *huh.Theme) (*huh.Form, *slackTokenValue) {
	tok := &slackTokenValue{}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Slack User Token").
				Description("Starts with xoxp- — from api.slack.com/apps > OAuth & Permissions > User OAuth Token").
				Value(&tok.Token).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("required")
					}
					return nil
				}),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(theme)

	return form, tok
}
