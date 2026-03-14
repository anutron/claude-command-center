package settings

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/auth"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// datasourceRecheckResult is a tea.Msg carrying the result of a live re-check.
type datasourceRecheckResult struct {
	Slug   string
	Result plugin.ValidationResult
	Live   bool // true when the result came from a real API call (not just structural check)
}

// datasourceFormValues holds the selected action from a datasource pane form.
type datasourceFormValues struct {
	Action string
}

// --- Data source form ---

func (p *Plugin) buildDatasourceForm(item *NavItem) *huh.Form {
	p.datasourceValues = &datasourceFormValues{}

	// Build contextual action options
	options := []huh.Option[string]{
		huh.NewOption("Verify credentials", "recheck"),
	}
	if isGoogleDatasource(item.Slug) {
		options = append(options,
			huh.NewOption("Authenticate (enter client credentials + OAuth)", "auth"),
			huh.NewOption("Open Google Cloud Console", "console"),
		)
	} else if item.Slug == "slack" {
		options = append(options,
			huh.NewOption("Enter Slack token", "auth"),
		)
	}

	vals := p.datasourceValues

	// The form contains only the validation status Note and action Select.
	// Provider views (calendar list, GitHub repos, etc.) are rendered
	// separately above the form in viewContent so they remain interactive —
	// wrapping them in a huh.Note made them read-only (BUG-050).
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				DescriptionFunc(func() string {
					return p.viewValidationStatus(item)
				}, &vals.Action),
			huh.NewSelect[string]().
				Title("ACTIONS").
				Options(options...).
				Value(&p.datasourceValues.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handleDatasourceFormCompletion(slug string) tea.Cmd {
	if p.datasourceValues == nil {
		return nil
	}
	action := p.datasourceValues.Action
	p.datasourceValues = nil

	item := p.findNavItem(slug)

	switch action {
	case "recheck":
		var cmds []tea.Cmd

		// Let the provider handle recheck for provider-specific refresh logic.
		if sp, ok := p.providers[slug]; ok {
			provAction := sp.HandleSettingsKey(tea.KeyMsg{})
			if provAction.TeaCmd != nil {
				cmds = append(cmds, provAction.TeaCmd)
			}
		}

		// Always run a live credential recheck (hits the actual API).
		cmds = append(cmds, func() tea.Msg {
			result := p.validateDataSourceResult(slug, true)
			return datasourceRecheckResult{Slug: slug, Result: result, Live: true}
		})

		label := slug
		if item != nil {
			label = item.Label
		}
		p.flashMessage = "Verifying " + label + " credentials..."
		p.flashMessageAt = currentTime()

		// Rebuild form so it stays on screen
		if item != nil {
			form := p.buildDatasourceForm(item)
			p.activeForm = form
			p.activeFormSlug = slug
			cmds = append(cmds, form.Init())
		}

		return tea.Batch(cmds...)

	case "auth":
		if slug == "slack" {
			form, tok := newSlackTokenForm(p.styles.huhTheme)
			p.activeForm = form
			p.activeFormSlug = slug
			p.pendingSlackToken = tok
			p.pendingAuthSlug = slug
			p.focusZone = FocusForm
			initCmd := p.activeForm.Init()
			p.flashMessage = "Enter Slack token"
			p.flashMessageAt = currentTime()
			return initCmd
		}
		if isGoogleDatasource(slug) {
			form, creds := newClientCredForm(p.styles.huhTheme)
			p.activeForm = form
			p.activeFormSlug = slug
			p.pendingAuthCreds = creds
			p.pendingAuthSlug = slug
			p.focusZone = FocusForm
			initCmd := p.activeForm.Init()
			label := slug
			if item != nil {
				label = item.Label
			}
			p.flashMessage = "Enter OAuth client credentials for " + label
			p.flashMessageAt = currentTime()
			return initCmd
		}

	case "console":
		if isGoogleDatasource(slug) {
			_ = exec.Command("open", "https://console.cloud.google.com/apis/credentials").Start()
			p.flashMessage = "Opening Google Cloud Console..."
			p.flashMessageAt = currentTime()
			// Rebuild form so it stays on screen
			if item != nil {
				form := p.buildDatasourceForm(item)
				p.activeForm = form
				p.activeFormSlug = slug
				return form.Init()
			}
		}
	}

	return nil
}

// viewValidationStatus renders the tiered credential validation status and instructions.
func (p *Plugin) viewValidationStatus(item *NavItem) string {
	var lines []string

	greenCheck := lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("\u2713")
	yellowWarn := lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68")).Render("\u26a0")
	redX := lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("\u2717")

	switch item.ValidationStatus {
	case "ok":
		label := "Token verified"
		if item.SyncStatus != nil && item.SyncStatus.LastSuccess != nil {
			label = "Token verified — last sync " + db.RelativeTime(*item.SyncStatus.LastSuccess)
		}
		lines = append(lines, fmt.Sprintf("%s %s",
			greenCheck,
			p.styles.enabled.Render(label)))
		if item.ValidationMsg != "" {
			lines = append(lines, p.styles.muted.Render(item.ValidationMsg))
		}

	case "unverified":
		lines = append(lines, fmt.Sprintf("%s %s",
			yellowWarn,
			p.styles.logWarn.Render("Token not yet verified")))
		if item.ValidationMsg != "" {
			lines = append(lines, p.styles.muted.Render(item.ValidationMsg))
		}
		if item.ValidHint != "" {
			lines = append(lines, "")
			lines = append(lines, p.styles.logWarn.Render("Next: ")+p.styles.muted.Render(item.ValidHint))
		}

	case "incomplete":
		lines = append(lines, fmt.Sprintf("%s %s",
			yellowWarn,
			p.styles.logWarn.Render("Credentials incomplete")))
		if item.ValidationMsg != "" {
			lines = append(lines, p.styles.muted.Render(item.ValidationMsg))
		}
		if item.ValidHint != "" {
			lines = append(lines, "")
			lines = append(lines, p.styles.logWarn.Render("Fix: ")+p.styles.muted.Render(item.ValidHint))
		}

	case "no_client":
		lines = append(lines, fmt.Sprintf("%s %s",
			redX,
			p.styles.logError.Render("OAuth client credentials missing")))
		if item.ValidationMsg != "" {
			lines = append(lines, p.styles.muted.Render(item.ValidationMsg))
		}
		lines = append(lines, "")
		lines = append(lines, p.styles.muted.Render("To fix:"))
		lines = append(lines, p.styles.muted.Render("1. Create OAuth credentials in Google Cloud Console"))
		lines = append(lines, p.styles.muted.Render("2. Add clientId and clientSecret to your credential file"))
		if item.ValidHint != "" {
			lines = append(lines, p.styles.muted.Render("   "+item.ValidHint))
		}

	case "missing":
		lines = append(lines, fmt.Sprintf("%s %s",
			redX,
			p.styles.logError.Render("Credentials not found")))
		if item.ValidationMsg != "" {
			lines = append(lines, p.styles.muted.Render(item.ValidationMsg))
		}
		if item.ValidHint != "" {
			lines = append(lines, "")
			lines = append(lines, p.styles.muted.Render("Setup: ")+p.styles.muted.Render(item.ValidHint))
		}

	default:
		// Legacy fallback
		if item.Valid != nil {
			if *item.Valid {
				lines = append(lines, fmt.Sprintf("%s %s",
					p.styles.muted.Render("Credentials:"),
					p.styles.enabled.Render("Valid")))
			} else {
				lines = append(lines, fmt.Sprintf("%s %s",
					p.styles.muted.Render("Credentials:"),
					p.styles.logError.Render("Invalid")))
				if item.ValidHint != "" {
					lines = append(lines, fmt.Sprintf("%s %s",
						p.styles.muted.Render("Hint:"),
						p.styles.logWarn.Render(item.ValidHint)))
				}
			}
		}
	}

	// Show last sync status
	if item.SyncStatus != nil {
		lines = append(lines, "")
		ss := item.SyncStatus
		if ss.LastSuccess != nil {
			syncTime := db.RelativeTime(*ss.LastSuccess)
			lines = append(lines, fmt.Sprintf("%s %s",
				p.styles.muted.Render("Last sync:"),
				p.styles.enabled.Render(syncTime)))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s",
				p.styles.muted.Render("Last sync:"),
				p.styles.logError.Render("Never")))
		}
		if ss.LastError != "" {
			lines = append(lines, fmt.Sprintf("%s %s",
				p.styles.muted.Render("Last error:"),
				p.styles.logError.Render(ss.LastError)))
		}
	} else {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s %s",
			p.styles.muted.Render("Last sync:"),
			p.styles.logWarn.Render("Never synced")))
	}

	return strings.Join(lines, "\n")
}

// isGoogleDatasource returns true for data sources that use Google OAuth.
func isGoogleDatasource(slug string) bool {
	return slug == "calendar" || slug == "gmail"
}

// startAuthFlowForDatasource begins the OAuth flow for the pending data source
// using the credentials collected from the form.
func (p *Plugin) startAuthFlowForDatasource() tea.Cmd {
	creds := p.pendingAuthCreds
	slug := p.pendingAuthSlug
	if creds == nil || slug == "" {
		return nil
	}

	// Validate credentials before launching OAuth flow.
	if err := auth.ValidateClientCredentials(creds.ClientID); err != nil {
		p.flashMessage = "Invalid credentials: " + err.Error()
		p.flashMessageAt = time.Now()
		return nil
	}

	// Build the OAuth config and token path based on the data source.
	oauthConf, tokenPath := p.oauthConfigForSlug(slug, creds.ClientID, creds.ClientSecret)
	if oauthConf == nil {
		return nil
	}

	// Create a cancellable context for the auth flow.
	ctx, cancel := context.WithCancel(context.Background())
	p.authCancel = cancel

	p.flashMessage = "Waiting for OAuth callback... (press esc to cancel)"
	p.flashMessageAt = time.Now()

	return authFlowCmdFunc(ctx, oauthConf, tokenPath, creds.ClientID, creds.ClientSecret)
}
