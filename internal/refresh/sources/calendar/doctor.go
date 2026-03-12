package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
)

// ValidateCalendarResult performs a structural check on Calendar credentials.
func ValidateCalendarResult() plugin.ValidationResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "Cannot determine home directory",
			Hint:    fmt.Sprintf("Error: %v", err),
		}
	}

	dir := filepath.Join(home, ".config", "google-calendar-mcp")
	credsPath := filepath.Join(dir, "credentials.json")
	tokenPath := filepath.Join(dir, "token.json")

	// Check credentials.json first (new format)
	if data, err := os.ReadFile(credsPath); err == nil {
		var creds map[string]interface{}
		if err := json.Unmarshal(data, &creds); err != nil {
			return plugin.ValidationResult{
				Status:  "incomplete",
				Message: "credentials.json is malformed",
				Hint:    "Re-run calendar auth to regenerate credentials",
			}
		}
		// Check for required fields
		if _, ok := creds["clientId"]; !ok {
			if _, ok2 := creds["access_token"]; !ok2 {
				return plugin.ValidationResult{
					Status:  "incomplete",
					Message: "credentials.json missing required fields",
					Hint:    "Re-run calendar auth to regenerate credentials",
				}
			}
		}
		return plugin.ValidationResult{
			Status:  "ok",
			Message: "Calendar credentials found",
		}
	}

	// Fallback: check token.json (old format)
	if _, err := os.Stat(tokenPath); err == nil {
		return plugin.ValidationResult{
			Status:  "ok",
			Message: "Calendar token found (legacy format)",
			Hint:    "Consider migrating to credentials.json",
		}
	}

	return plugin.ValidationResult{
		Status:  "missing",
		Message: "No calendar credentials found",
		Hint:    "Place credentials.json in ~/.config/google-calendar-mcp/",
	}
}

// DoctorChecks implements plugin.DoctorProvider for Calendar.
func (s *Settings) DoctorChecks(opts plugin.DoctorOpts) []plugin.DoctorCheck {
	result := ValidateCalendarResult()
	checks := []plugin.DoctorCheck{
		{
			Name:   "Calendar credentials",
			Result: result,
		},
	}

	// Live token check: hit Google tokeninfo endpoint
	if opts.Live && result.Status == "ok" {
		checks = append(checks, liveCalendarTokenCheck())
	}

	return checks
}

// liveCalendarTokenCheck verifies the calendar token is valid by hitting Google's tokeninfo endpoint.
func liveCalendarTokenCheck() plugin.DoctorCheck {
	ts, err := loadCalendarAuth()
	if err != nil {
		return plugin.DoctorCheck{
			Name: "Calendar token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: "Cannot load calendar auth",
				Hint:    err.Error(),
			},
		}
	}

	tok, err := ts.Token()
	if err != nil {
		return plugin.DoctorCheck{
			Name: "Calendar token (live)",
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
			Name: "Calendar token (live)",
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
			Name: "Calendar token (live)",
			Result: plugin.ValidationResult{
				Status:  "incomplete",
				Message: fmt.Sprintf("Token invalid (HTTP %d)", resp.StatusCode),
				Hint:    "Re-run calendar auth to refresh credentials",
			},
		}
	}

	return plugin.DoctorCheck{
		Name: "Calendar token (live)",
		Result: plugin.ValidationResult{
			Status:  "ok",
			Message: "Calendar token is valid",
		},
	}
}
