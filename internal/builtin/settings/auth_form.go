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

// newClientCredForm creates a huh form for entering Google OAuth client credentials.
// The form collects a Client ID and Client Secret, both required non-empty.
func newClientCredForm() (*huh.Form, *clientCredentials) {
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
	).WithShowHelp(false).WithShowErrors(true)

	return form, creds
}
