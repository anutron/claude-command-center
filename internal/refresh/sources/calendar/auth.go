package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/refresh"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gcal "google.golang.org/api/calendar/v3"
)

// LoadAuth returns an authenticated TokenSource for Google Calendar.
// It wraps the internal auth loading logic for use by external packages.
func LoadAuth() (oauth2.TokenSource, error) {
	return loadCalendarAuth()
}

func loadCalendarAuth() (oauth2.TokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "google-calendar-mcp")

	credsPath := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")

	var tf refresh.GoogleTokenFile
	var clientID, clientSecret string

	if data, err := os.ReadFile(credsPath); err == nil {
		if err := json.Unmarshal(data, &tf); err == nil && tf.ClientID != "" {
			clientID = tf.ClientID
			clientSecret = tf.ClientSecret
		}
	}

	if clientID == "" {
		data, err := os.ReadFile(tokenPath)
		if err != nil {
			return nil, fmt.Errorf("no calendar token found at %s or %s: %w", credsPath, tokenPath, err)
		}
		if err := json.Unmarshal(data, &tf); err != nil {
			return nil, fmt.Errorf("parsing calendar token: %w", err)
		}
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
		if clientID == "" || clientSecret == "" {
			clientID, clientSecret = refresh.LoadCalendarCredsFromClaudeConfig()
		}
		if clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("no Google Calendar client credentials: set GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET or migrate to credentials.json")
		}
	}

	conf := refresh.LoadGoogleOAuth2Config(clientID, clientSecret, gcal.CalendarScope, gcal.CalendarEventsScope)
	tok := tf.ToOAuth2Token()
	return conf.TokenSource(context.Background(), tok), nil
}

// RunCalendarAuth performs the OAuth2 flow for Google Calendar.
func RunCalendarAuth() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "google-calendar-mcp")

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" {
		clientID, clientSecret = refresh.LoadCalendarCredsFromClaudeConfig()
	}
	if clientID == "" {
		return fmt.Errorf("no Google Calendar client credentials found")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gcal.CalendarScope, gcal.CalendarEventsScope},
		RedirectURL:  "http://localhost:3000/oauth2callback",
	}

	url := conf.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	fmt.Printf("Open this URL in your browser:\n%s\n\nWaiting for callback on http://localhost:3000...\n", url)

	_ = exec.Command("open", url).Run()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code", 400)
			errCh <- fmt.Errorf("no authorization code received")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<h1>Authorized!</h1><p>You can close this tab.</p>")
		codeCh <- code
	})

	srv := &http.Server{Addr: ":3000", Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("listen on :3000: %w", err)
		}
	}()

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		srv.Close()
		return err
	case <-time.After(5 * time.Minute):
		srv.Close()
		return fmt.Errorf("timeout waiting for authorization")
	}
	srv.Close()

	tok, err := conf.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}
	creds := refresh.GoogleTokenFile{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiryDate:   tok.Expiry.UnixMilli(),
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	return os.WriteFile(filepath.Join(dir, "credentials.json"), data, 0o600)
}

// MigrateCalendarCredentials migrates token.json to credentials.json format.
func MigrateCalendarCredentials() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "google-calendar-mcp")
	credsPath := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")

	if _, err := os.Stat(credsPath); err == nil {
		return nil
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("no token.json to migrate: %w", err)
	}

	var tf refresh.GoogleTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return fmt.Errorf("parsing token.json: %w", err)
	}

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" {
		clientID, clientSecret = refresh.LoadCalendarCredsFromClaudeConfig()
	}
	if clientID == "" {
		return fmt.Errorf("cannot migrate: no Google Calendar client credentials found")
	}

	tf.ClientID = clientID
	tf.ClientSecret = clientSecret

	out, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(credsPath, out, 0o600)
}
