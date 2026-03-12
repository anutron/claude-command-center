package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/oauth2"
)

// AuthFlowResultMsg is the tea.Msg returned when the OAuth flow completes.
type AuthFlowResultMsg struct {
	Token *oauth2.Token
	Error error
}

// AuthFlowOpts configures a browser-based OAuth2 flow.
type AuthFlowOpts struct {
	Config       *oauth2.Config // OAuth2 config (must have ClientID, ClientSecret, Scopes set)
	TokenPath    string         // Where to save the token file (e.g. ~/.config/google-calendar-mcp/credentials.json)
	ClientID     string         // Client ID to embed in saved token file
	ClientSecret string         // Client secret to embed in saved token file
	Timeout      time.Duration  // Max time to wait for callback (default 5 minutes)
}

// AuthFlowCmd returns a tea.Cmd that runs a browser-based OAuth2 flow.
// It starts a local HTTP server on a random port, opens the browser to the
// authorization URL, waits for the callback, exchanges the code for a token,
// saves it to opts.TokenPath, and returns an AuthFlowResultMsg.
//
// The provided context can be cancelled (e.g. on esc) to abort the flow.
func AuthFlowCmd(ctx context.Context, opts AuthFlowOpts) tea.Cmd {
	return func() tea.Msg {
		tok, err := runAuthFlow(ctx, opts)
		return AuthFlowResultMsg{Token: tok, Error: err}
	}
}

func runAuthFlow(ctx context.Context, opts AuthFlowOpts) (*oauth2.Token, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	// Start listener on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/oauth2callback", port)

	// Set the redirect URL on the config.
	conf := *opts.Config
	conf.RedirectURL = redirectURL

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no authorization code", http.StatusBadRequest)
			errCh <- fmt.Errorf("no authorization code received")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<h1>Authorized!</h1><p>You can close this tab and return to CCC.</p>")
		codeCh <- code
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("serve: %w", err)
		}
	}()
	defer srv.Close()

	// Open browser.
	url := conf.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	_ = exec.Command("open", url).Start()

	// Wait for callback, context cancellation, or timeout.
	timer := time.NewTimer(opts.Timeout)
	defer timer.Stop()

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("auth flow cancelled")
	case <-timer.C:
		return nil, fmt.Errorf("timeout waiting for authorization")
	}

	// Exchange code for token.
	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	// Save token file.
	if opts.TokenPath != "" {
		if err := saveTokenFile(opts, tok); err != nil {
			return tok, fmt.Errorf("save token: %w", err)
		}
	}

	return tok, nil
}

// saveTokenFile writes the token to disk in GoogleTokenFile format.
func saveTokenFile(opts AuthFlowOpts, tok *oauth2.Token) error {
	dir := filepath.Dir(opts.TokenPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tf := GoogleTokenFile{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
	}
	if !tok.Expiry.IsZero() {
		tf.ExpiryDate = tok.Expiry.UnixMilli()
	}

	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(opts.TokenPath, data, 0o600)
}
