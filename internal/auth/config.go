package auth

import (
	"encoding/json"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// LoadGoogleOAuth2Config creates an OAuth2 config for Google APIs.
func LoadGoogleOAuth2Config(clientID, clientSecret string, scopes ...string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
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
