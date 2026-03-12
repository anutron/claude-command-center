package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// GmailDoctor implements plugin.DoctorProvider for Gmail.
type GmailDoctor struct {
	cfg config.GmailConfig
}

// NewDoctor creates a GmailDoctor with the given config.
func NewDoctor(cfg config.GmailConfig) *GmailDoctor {
	return &GmailDoctor{cfg: cfg}
}

// ValidateGmailResult performs a structural check on Gmail credentials.
func ValidateGmailResult() plugin.ValidationResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "Cannot determine home directory",
			Hint:    fmt.Sprintf("Error: %v", err),
		}
	}

	tokenPath := filepath.Join(home, ".gmail-mcp", "work.json")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "Gmail credentials not found",
			Hint:    fmt.Sprintf("Expected token at %s — run Gmail MCP auth to configure", tokenPath),
		}
	}

	var token map[string]interface{}
	if err := json.Unmarshal(data, &token); err != nil {
		return plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Gmail credentials malformed",
			Hint:    "Re-run Gmail MCP auth to reconfigure",
		}
	}

	// Check for client credentials
	if _, ok := token["clientId"]; !ok {
		// Check env fallback
		if os.Getenv("GMAIL_CLIENT_ID") == "" {
			return plugin.ValidationResult{
				Status:  "no_client",
				Message: "Gmail token missing client credentials",
				Hint:    "Set GMAIL_CLIENT_ID env or add clientId to work.json",
			}
		}
	}

	return plugin.ValidationResult{
		Status:  "ok",
		Message: "Gmail credentials found",
	}
}

// DoctorChecks implements plugin.DoctorProvider for Gmail.
func (d *GmailDoctor) DoctorChecks(opts plugin.DoctorOpts) []plugin.DoctorCheck {
	result := ValidateGmailResult()
	checks := []plugin.DoctorCheck{
		{
			Name:   "Gmail credentials",
			Result: result,
		},
	}

	// Live token check: hit Google tokeninfo endpoint
	if opts.Live && result.Status == "ok" {
		checks = append(checks, liveGmailTokenCheck(d.cfg.Advanced))
	}

	return checks
}

// liveGmailTokenCheck verifies the Gmail token is valid by hitting Google's tokeninfo endpoint.
func liveGmailTokenCheck(advanced bool) plugin.DoctorCheck {
	ts, err := loadGmailAuth(advanced)
	if err != nil {
		return plugin.DoctorCheck{
			Name: "Gmail token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: "Cannot load Gmail auth",
				Hint:    err.Error(),
			},
		}
	}

	tok, err := ts.Token()
	if err != nil {
		return plugin.DoctorCheck{
			Name: "Gmail token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: "Cannot obtain token",
				Hint:    err.Error(),
			},
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo?access_token=" + tok.AccessToken)
	if err != nil {
		return plugin.DoctorCheck{
			Name: "Gmail token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: "Cannot reach Google tokeninfo endpoint",
				Hint:    err.Error(),
			},
			Inconclusive: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return plugin.DoctorCheck{
			Name: "Gmail token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: fmt.Sprintf("Token invalid (HTTP %d)", resp.StatusCode),
				Hint:    "Re-run Gmail MCP auth to refresh credentials",
			},
		}
	}

	return plugin.DoctorCheck{
		Name: "Gmail token (live)",
		Result: plugin.ValidationResult{
			Status:  "ok",
			Message: "Gmail token is valid",
		},
	}
}
