package refresh

import (
	"os"
	"strings"
	"time"

	"encoding/json"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleTokenFile represents a stored OAuth2 token file used by Google APIs.
type GoogleTokenFile struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiryDate   int64  `json:"expiry_date,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ToOAuth2Token converts the stored token file to an oauth2.Token.
func (g *GoogleTokenFile) ToOAuth2Token() *oauth2.Token {
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

// LoadGoogleOAuth2Config creates an OAuth2 config for Google APIs.
func LoadGoogleOAuth2Config(clientID, clientSecret string, scopes ...string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
	}
}

// LoadEnvFile reads KEY=VALUE pairs from ~/.config/ccc/.env and sets them as env vars.
func LoadEnvFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(home + "/.config/ccc/.env")
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

// LoadCalendarCredsFromClaudeConfig reads Google Calendar client credentials
// from ~/.claude.json MCP server config.
func LoadCalendarCredsFromClaudeConfig() (clientID, clientSecret string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(home + "/.claude.json")
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
