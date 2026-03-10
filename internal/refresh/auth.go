package refresh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
)

type googleTokenFile struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiryDate   int64  `json:"expiry_date,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

func (g *googleTokenFile) toOAuth2Token() *oauth2.Token {
	tok := &oauth2.Token{
		AccessToken:  g.AccessToken,
		RefreshToken: g.RefreshToken,
		TokenType:    g.TokenType,
	}
	if g.ExpiryDate > 0 {
		tok.Expiry = time.UnixMilli(g.ExpiryDate)
	}
	return tok
}

func loadGoogleOAuth2Config(clientID, clientSecret string, scopes ...string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
	}
}

func loadCalendarAuth() (oauth2.TokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "google-calendar-mcp")

	credsPath := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")

	var tf googleTokenFile
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
			clientID, clientSecret = loadCalendarCredsFromClaudeConfig()
		}
		if clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("no Google Calendar client credentials: set GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET or migrate to credentials.json")
		}
	}

	conf := loadGoogleOAuth2Config(clientID, clientSecret, calendar.CalendarScope, calendar.CalendarEventsScope)
	tok := tf.toOAuth2Token()
	return conf.TokenSource(context.Background(), tok), nil
}

func loadGmailAuth() (oauth2.TokenSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".gmail-mcp", "work.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no gmail token at %s: %w", path, err)
	}

	var tf googleTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, fmt.Errorf("parsing gmail token: %w", err)
	}

	clientID := tf.ClientID
	clientSecret := tf.ClientSecret
	if clientID == "" {
		clientID = os.Getenv("GMAIL_CLIENT_ID")
		clientSecret = os.Getenv("GMAIL_CLIENT_SECRET")
	}
	if clientID == "" {
		return nil, fmt.Errorf("gmail token missing clientId and GMAIL_CLIENT_ID not set")
	}

	conf := loadGoogleOAuth2Config(clientID, clientSecret, gmail.GmailReadonlyScope)
	tok := tf.toOAuth2Token()
	return conf.TokenSource(context.Background(), tok), nil
}

func loadSlackToken() (string, error) {
	tok := os.Getenv("SLACK_BOT_TOKEN")
	if tok == "" {
		return "", fmt.Errorf("SLACK_BOT_TOKEN not set")
	}
	return tok, nil
}

type granolaStoredAccounts struct {
	Accounts []granolaAccount `json:"accounts"`
}

type granolaAccount struct {
	Email       string            `json:"email"`
	AccessToken string            `json:"access_token"`
	ObtainedAt  int64             `json:"obtained_at"`
	ExpiresIn   int64             `json:"expires_in"`
	Tokens      map[string]string `json:"tokens"`
}

func loadGranolaAuth() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, "Library", "Application Support", "Granola", "stored-accounts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no granola auth at %s: %w", path, err)
	}

	var wrapper struct {
		Accounts string `json:"accounts"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("parsing granola stored-accounts.json: %w", err)
	}

	var accounts []struct {
		UserID  string `json:"userId"`
		Email   string `json:"email"`
		Tokens  string `json:"tokens"`
		SavedAt int64  `json:"savedAt"`
	}
	if err := json.Unmarshal([]byte(wrapper.Accounts), &accounts); err != nil {
		return "", fmt.Errorf("parsing granola accounts array: %w", err)
	}

	if len(accounts) == 0 {
		return "", fmt.Errorf("no granola accounts found")
	}

	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal([]byte(accounts[0].Tokens), &tokens); err != nil {
		return "", fmt.Errorf("parsing granola tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return "", fmt.Errorf("granola access token is empty")
	}

	if accounts[0].SavedAt > 0 && tokens.ExpiresIn > 0 {
		savedAt := time.UnixMilli(accounts[0].SavedAt)
		expiresAt := savedAt.Add(time.Duration(tokens.ExpiresIn) * time.Second)
		if time.Now().After(expiresAt) {
			return "", fmt.Errorf("granola token expired at %s — open Granola app to refresh", expiresAt.Format(time.RFC3339))
		}
	}

	return tokens.AccessToken, nil
}

func loadGitHubToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
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
		clientID, clientSecret = loadCalendarCredsFromClaudeConfig()
	}
	if clientID == "" {
		return fmt.Errorf("no Google Calendar client credentials found")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{calendar.CalendarScope, calendar.CalendarEventsScope},
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
	creds := googleTokenFile{
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

// loadEnvFile reads KEY=VALUE pairs from ~/.config/ccc/.env and sets them as env vars.
func loadEnvFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "ccc", ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func loadCalendarCredsFromClaudeConfig() (clientID, clientSecret string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return "", ""
	}

	var config struct {
		MCPServers map[string]struct {
			Env map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", ""
	}

	if cal, ok := config.MCPServers["google-calendar"]; ok {
		return cal.Env["GOOGLE_CLIENT_ID"], cal.Env["GOOGLE_CLIENT_SECRET"]
	}
	return "", ""
}

func migrateCalendarCredentials() error {
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

	var tf googleTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return fmt.Errorf("parsing token.json: %w", err)
	}

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" {
		clientID, clientSecret = loadCalendarCredsFromClaudeConfig()
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
