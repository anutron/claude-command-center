// Package settings implements the Settings plugin for CCC.
// It provides a sidebar-based UI for appearance, plugins, data sources,
// system actions, and logs.
package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"database/sql"

	"github.com/anutron/claude-command-center/internal/auth"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/refresh/sources/calendar"
	ghsettings "github.com/anutron/claude-command-center/internal/refresh/sources/github"
	"github.com/anutron/claude-command-center/internal/refresh/sources/gmail"
	"github.com/anutron/claude-command-center/internal/refresh/sources/granola"
	"github.com/anutron/claude-command-center/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/oauth2"
	gcal "google.golang.org/api/calendar/v3"
	gm "google.golang.org/api/gmail/v1"
)

// Plugin implements the plugin.Plugin interface for Settings.
type Plugin struct {
	cfg      *config.Config
	database *sql.DB
	logger   plugin.Logger
	registry *plugin.Registry
	bus      plugin.EventBus
	styles   settingsStyles

	// Shared style pointers from plugin.Context — updated in-place on palette change
	// so the TUI host and all plugins see the new styles immediately.
	sharedStyles *ui.Styles
	sharedGrad   *ui.GradientColors

	// SettingsProvider implementations for data sources
	providers map[string]plugin.SettingsProvider

	// Sidebar nav state
	navCategories []Category
	navCursor     int
	focusZone     FocusZone

	// Banner editing state (used by content_appearance)
	bannerNameInput     textinput.Model
	bannerSubtitleInput textinput.Model
	bannerField         int  // 0=name, 1=subtitle, 2=show/hide, 3=top padding
	bannerEditing       bool // true when a text field is focused
	bannerPaddingInput  textinput.Model

	// Palette state (used by content_appearance)
	paletteCursor int

	// Logs state (used by content_logs)
	logOffset      int
	logFilterInput textinput.Model
	logFilterMode  bool // true when filter input is focused

	// System content pane cursor positions (keyed by slug)
	systemCursors map[string]int

	// Active huh form (nil when no form is displayed)
	activeForm *huh.Form

	// Pending OAuth auth state
	pendingAuthCreds *clientCredentials // credentials from the form
	pendingAuthSlug  string            // data source slug being authenticated
	authCancel       context.CancelFunc // cancel function for in-progress OAuth flow

	// Pending Slack token state
	pendingSlackToken *slackTokenValue

	// Flash message
	flashMessage   string
	flashMessageAt time.Time

	width, height int
}

// New creates a new Settings plugin. The registry is used to enumerate all plugins.
func New(registry *plugin.Registry) *Plugin {
	return &Plugin{
		registry: registry,
	}
}

func (p *Plugin) Slug() string    { return "settings" }
func (p *Plugin) TabName() string { return "Settings" }

func (p *Plugin) Migrations() []plugin.Migration { return nil }

func (p *Plugin) Routes() []plugin.Route {
	return []plugin.Route{
		{Slug: "settings", Description: "Settings"},
	}
}

func (p *Plugin) NavigateTo(route string, args map[string]string) {
	// The sidebar layout handles all navigation internally.
	// External navigation just activates the settings tab.
}

func (p *Plugin) RefreshInterval() time.Duration { return 0 }
func (p *Plugin) Refresh() tea.Cmd               { return nil }
func (p *Plugin) Shutdown()                       {}

func (p *Plugin) Init(ctx plugin.Context) error {
	p.cfg = ctx.Config
	p.database = ctx.DB
	p.logger = ctx.Logger
	p.bus = ctx.Bus
	p.sharedStyles = ctx.Styles
	p.sharedGrad = ctx.Grad

	pal := config.GetPalette(p.cfg.Palette, p.cfg.Colors)
	p.styles = newSettingsStyles(pal)

	// Set palette cursor to current palette
	for i, name := range config.PaletteNames() {
		if name == p.cfg.Palette {
			p.paletteCursor = i
			break
		}
	}

	// Initialize providers map and register data source settings providers.
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers["calendar"] = calendar.NewSettings(p.cfg, pal, p.logger)
	p.providers["github"] = ghsettings.NewSettings(p.cfg, pal, p.logger)
	p.providers["granola"] = granola.NewSettings(p.cfg, pal)

	// Banner text inputs
	ni := textinput.New()
	ni.Placeholder = "Claude Command"
	ni.CharLimit = 20
	ni.SetValue(p.cfg.Name)
	p.bannerNameInput = ni

	si := textinput.New()
	si.Placeholder = "Center"
	si.CharLimit = 30
	si.SetValue(p.cfg.Subtitle)
	p.bannerSubtitleInput = si

	pi := textinput.New()
	pi.Placeholder = "10"
	pi.CharLimit = 3
	pi.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
	p.bannerPaddingInput = pi

	// Log filter input
	fi := textinput.New()
	fi.Placeholder = "filter logs..."
	fi.CharLimit = 100
	fi.Prompt = "/ "
	p.logFilterInput = fi

	// Build sidebar navigation
	p.rebuildNav()

	// Subscribe to todo events for logging
	if p.bus != nil {
		todoTopics := []string{"todo.completed", "todo.created", "todo.dismissed", "todo.deferred", "todo.promoted", "todo.edited"}
		for _, topic := range todoTopics {
			t := topic // capture
			p.bus.Subscribe(t, func(e plugin.Event) {
				if p.logger != nil {
					if m, ok := e.Payload.(map[string]interface{}); ok {
						title, _ := m["title"].(string)
						p.logger.Info("settings", fmt.Sprintf("event %s: %s", t, title))
					}
				}
			})
		}
	}

	return nil
}

// RegisterProvider adds a SettingsProvider for a given slug.
// This allows data source packages to provide their own settings UI.
func (p *Plugin) RegisterProvider(slug string, sp plugin.SettingsProvider) {
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers[slug] = sp
}

// StartCmds returns initial commands (none needed for settings).
func (p *Plugin) StartCmds() tea.Cmd { return nil }

func (p *Plugin) KeyBindings() []plugin.KeyBinding {
	return []plugin.KeyBinding{
		{Key: "up/down", Description: "Navigate", Promoted: true},
		{Key: "enter/right", Description: "Open content pane", Promoted: true},
		{Key: "esc/left", Description: "Back to sidebar", Promoted: true},
		{Key: "space", Description: "Toggle enable/disable", Promoted: true},
	}
}

func (p *Plugin) HandleKey(msg tea.KeyMsg) plugin.Action {
	switch p.focusZone {
	case FocusNav:
		return p.handleNavKey(msg)
	case FocusContent, FocusEditing:
		return p.handleContentKey(msg)
	case FocusForm:
		return p.handleFormKey(msg)
	}
	return plugin.NoopAction()
}

// handleFormKey processes key events when a huh form is focused.
func (p *Plugin) handleFormKey(msg tea.KeyMsg) plugin.Action {
	if p.activeForm == nil {
		p.focusZone = FocusContent
		return plugin.NoopAction()
	}

	// Allow escape to cancel the form and any pending auth
	if msg.String() == "esc" {
		p.activeForm = nil
		p.focusZone = FocusContent
		p.cancelAuthFlow()
		p.pendingAuthCreds = nil
		p.pendingSlackToken = nil
		p.pendingAuthSlug = ""
		return plugin.NoopAction()
	}

	// Forward key to the form
	form, cmd := p.activeForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		p.activeForm = f
	}

	// Check if form completed
	if p.activeForm.State == huh.StateCompleted {
		p.activeForm = nil
		p.focusZone = FocusContent

		// If this was a Slack token form, save and recheck.
		if p.pendingSlackToken != nil && p.pendingAuthSlug == "slack" {
			recheckCmd := p.saveSlackToken()
			if recheckCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: recheckCmd}
			}
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}

		// If this was a credential form for OAuth, chain to the auth flow.
		if p.pendingAuthCreds != nil && p.pendingAuthSlug != "" {
			authCmd := p.startAuthFlowForDatasource()
			if authCmd != nil {
				return plugin.Action{Type: plugin.ActionNoop, TeaCmd: authCmd}
			}
		}
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}

	if cmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	// Form is active but the key produced no cmd (e.g. Tab on a single-field
	// form). Return a consumed action so the TUI host doesn't switch tabs
	// while a form is visible (BUG-041).
	return plugin.ConsumedAction()
}

func (p *Plugin) HandleMessage(msg tea.Msg) (bool, plugin.Action) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return false, plugin.NoopAction()
	case datasourceRecheckResult:
		p.applyRecheckResult(msg)
		return true, plugin.NoopAction()
	case systemActionResult:
		p.handleSystemActionResult(msg)
		return true, plugin.NoopAction()
	case auth.AuthFlowResultMsg:
		cmd := p.handleAuthFlowResult(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	case plugin.TabLeaveMsg:
		// Cancel any active banner editing when leaving the tab
		if p.bannerEditing {
			p.bannerEditing = false
			p.bannerNameInput.SetValue(p.cfg.Name)
			p.bannerNameInput.Blur()
			p.bannerSubtitleInput.SetValue(p.cfg.Subtitle)
			p.bannerSubtitleInput.Blur()
			p.bannerPaddingInput.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
			p.bannerPaddingInput.Blur()
		}
		// Cancel any active form
		if p.activeForm != nil {
			p.activeForm = nil
			p.focusZone = FocusContent
		}
		// Cancel any in-progress OAuth flow and clear pending token state
		p.cancelAuthFlow()
		p.pendingSlackToken = nil
		return true, plugin.NoopAction()
	}

	// Route non-key messages to the active form when in FocusForm.
	if p.focusZone == FocusForm && p.activeForm != nil {
		form, cmd := p.activeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			p.activeForm = f
		}
		if p.activeForm.State == huh.StateCompleted {
			p.activeForm = nil
			p.focusZone = FocusContent

			// If this was a Slack token form, save and recheck.
			if p.pendingSlackToken != nil && p.pendingAuthSlug == "slack" {
				recheckCmd := p.saveSlackToken()
				if recheckCmd != nil {
					return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: recheckCmd}
				}
			}

			// Chain to OAuth flow if credential form completed.
			if p.pendingAuthCreds != nil && p.pendingAuthSlug != "" {
				authCmd := p.startAuthFlowForDatasource()
				if authCmd != nil {
					return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: authCmd}
				}
			}
		}
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	}

	// Route async result messages to the active provider regardless of focus
	// zone. The provider's HandleSettingsMsg only claims messages it owns
	// (e.g. CalendarFetchResultMsg, ghRepoFetchResult), so this is safe.
	// Without this, a fetch triggered from nav (via key forwarding) or one
	// whose result arrives after navigating back to nav would be silently
	// dropped, leaving fetchLoading=true forever (BUG-023, BUG-026).
	if sp := p.activeProvider(); sp != nil {
		if handled, action := sp.HandleSettingsMsg(msg); handled {
			if action.Type == plugin.ActionFlash {
				p.flashMessage = action.Payload
				p.flashMessageAt = time.Now()
				return true, plugin.NoopAction()
			}
			return true, action
		}
	}

	// Clear flash after 10 seconds
	if p.flashMessage != "" && time.Since(p.flashMessageAt) > 10*time.Second {
		p.flashMessage = ""
	}

	return false, plugin.NoopAction()
}

// activeProvider returns the SettingsProvider for the currently selected nav item,
// or nil if there is none.
func (p *Plugin) activeProvider() plugin.SettingsProvider {
	item := p.selectedNavItem()
	if item == nil {
		return nil
	}
	// Data source providers
	if sp, ok := p.providers[item.Slug]; ok {
		return sp
	}
	// Plugin providers (from registry)
	if item.Kind == "plugin" {
		if plug, ok := p.registry.BySlug(item.Slug); ok {
			if sp, ok := plug.(plugin.SettingsProvider); ok {
				return sp
			}
		}
	}
	return nil
}

// activeProviderOpenCmd returns the SettingsOpenCmd for the currently selected
// nav item's provider, or nil if there is none.
func (p *Plugin) activeProviderOpenCmd() tea.Cmd {
	if sp := p.activeProvider(); sp != nil {
		return sp.SettingsOpenCmd()
	}
	return nil
}

func (p *Plugin) View(width, height, frame int) string {
	p.syncNavFromConfig()
	p.width = width
	p.height = height

	viewWidth := ui.ContentMaxWidth
	if width > 0 && width < viewWidth {
		viewWidth = width
	}

	// Sidebar + content split
	sidebarWidth := 28
	contentWidth := viewWidth - sidebarWidth - 4 // account for borders
	if contentWidth < 20 {
		contentWidth = 20
	}
	panelHeight := height - 4 // leave room for help line + flash
	if panelHeight < 10 {
		panelHeight = 10
	}

	sidebar := p.viewSidebar(sidebarWidth, panelHeight, p.focusZone)
	content := p.viewContent(contentWidth, panelHeight)

	layout := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	// Flash message
	var flash string
	if p.flashMessage != "" {
		flash = p.styles.flash.Render("  > " + p.flashMessage)
	}

	// Help line
	var help string
	switch p.focusZone {
	case FocusNav:
		help = p.styles.muted.Render("  up/down navigate  space toggle  enter/right open  esc back")
	case FocusContent:
		item := p.selectedNavItem()
		if item != nil && item.Kind == "datasource" {
			if isGoogleDatasource(item.Slug) {
				help = p.styles.muted.Render("  esc/left sidebar  r re-check  a authenticate  o cloud console")
			} else if item.Slug == "slack" {
				help = p.styles.muted.Render("  esc/left sidebar  r re-check  a enter token")
			} else {
				help = p.styles.muted.Render("  esc/left sidebar  up/down navigate  enter select  space toggle  r re-check")
			}
		} else {
			help = p.styles.muted.Render("  esc/left sidebar  up/down navigate  enter select  space toggle")
		}
	case FocusEditing:
		help = p.styles.muted.Render("  enter save  esc cancel")
	case FocusForm:
		help = p.styles.muted.Render("  tab next field  enter submit  esc cancel")
	}

	parts := []string{layout}
	if flash != "" {
		parts = append(parts, flash)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// syncNavFromConfig updates nav item enabled states from the live config.
func (p *Plugin) syncNavFromConfig() {
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			item := &p.navCategories[i].Items[j]
			if item.Enabled == nil {
				continue
			}
			switch item.Kind {
			case "plugin":
				if item.Slug == "threads" {
					*item.Enabled = p.cfg.Threads.Enabled
				} else if len(item.Slug) > 9 && item.Slug[:9] == "external-" {
					// External plugins — find matching config entry
					for k, ep := range p.cfg.ExternalPlugins {
						if item.Slug == fmt.Sprintf("external-%d", k) {
							*item.Enabled = ep.Enabled
							break
						}
					}
				} else {
					*item.Enabled = p.cfg.PluginEnabled(item.Slug)
				}
			case "datasource":
				switch item.Slug {
				case "calendar":
					*item.Enabled = p.cfg.Calendar.Enabled
				case "github":
					*item.Enabled = p.cfg.GitHub.Enabled
				case "granola":
					*item.Enabled = p.cfg.Granola.Enabled
				case "slack":
					*item.Enabled = p.cfg.Slack.Enabled
				case "gmail":
					*item.Enabled = p.cfg.Gmail.Enabled
				case "todos":
					*item.Enabled = p.cfg.Todos.Enabled
				}
			}
		}
	}
}

// publishConfigSaved publishes a config.saved event via the bus.
func (p *Plugin) publishConfigSaved(keysChanged string) {
	if p.bus != nil {
		p.bus.Publish(plugin.Event{
			Source: "settings",
			Topic:  "config.saved",
			Payload: map[string]interface{}{
				"keys_changed": keysChanged,
			},
		})
	}
}

func (p *Plugin) validateDataSource(slug string) error {
	switch slug {
	case "calendar":
		return config.ValidateCalendar()
	case "github":
		return config.ValidateGitHub()
	case "granola":
		return config.ValidateGranola()
	case "slack":
		return config.ValidateSlack()
	case "gmail":
		return config.ValidateGmail()
	}
	return nil
}

// validateDataSourceResult returns a tiered ValidationResult for a data source.
// It prefers DoctorProvider checks from the provider, falling back to the
// top-level ValidateXResult functions for calendar/gmail, and then to the
// legacy error-based validateDataSource for others.
func (p *Plugin) validateDataSourceResult(slug string, live bool) plugin.ValidationResult {
	opts := plugin.DoctorOpts{Live: live}

	// Try DoctorProvider on the SettingsProvider first
	if sp, ok := p.providers[slug]; ok {
		if dp, ok := sp.(plugin.DoctorProvider); ok {
			checks := dp.DoctorChecks(opts)
			if len(checks) > 0 {
				// If live, prefer the live check result (last check) over structural
				if live && len(checks) > 1 {
					return checks[len(checks)-1].Result
				}
				return checks[0].Result
			}
		}
	}

	// Standalone DoctorProvider for Gmail (not in providers map)
	if slug == "gmail" {
		doc := gmail.NewDoctor(p.cfg.Gmail)
		checks := doc.DoctorChecks(opts)
		if len(checks) > 0 {
			if live && len(checks) > 1 {
				return checks[len(checks)-1].Result
			}
			return checks[0].Result
		}
		return gmail.ValidateGmailResult()
	}

	// Fallback to standalone ValidateXResult functions
	switch slug {
	case "calendar":
		return calendar.ValidateCalendarResult()
	case "slack":
		return validateSlackResult()
	}

	// Final fallback: legacy error-based check
	if err := p.validateDataSource(slug); err != nil {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: err.Error(),
			Hint:    err.Error(),
		}
	}
	return plugin.ValidationResult{
		Status:  "ok",
		Message: slug + " credentials configured",
	}
}

// applyRecheckResult updates the NavItem for a data source after a re-check.
func (p *Plugin) applyRecheckResult(msg datasourceRecheckResult) {
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			item := &p.navCategories[i].Items[j]
			if item.Slug == msg.Slug && item.Kind == "datasource" {
				item.ValidationStatus = msg.Result.Status
				item.ValidationMsg = msg.Result.Message
				item.ValidHint = msg.Result.Hint

				// Reload sync status from the database so the indicator
				// reflects actual sync results, not just credential checks.
				if p.database != nil {
					if syncMap, err := db.DBLoadAllSourceSync(p.database); err == nil && syncMap != nil {
						item.SyncStatus = syncMap[item.Slug]
					}
				}

				// Apply the same sync-aware downgrade as rebuildNav():
				// credentials may look "ok" structurally but if sync has
				// never succeeded or last sync failed, downgrade to
				// "incomplete" with a warning indicator (BUG-030).
				if msg.Result.Status == "ok" {
					ss := item.SyncStatus
					if ss == nil || ss.LastSuccess == nil {
						item.ValidationStatus = "unverified"
						item.ValidationMsg = "Token configured — run ccc-refresh to verify"
						item.ValidHint = "Run ccc-refresh or wait for next auto-refresh"
						v := false
						item.Valid = &v
					} else if ss.LastError != "" {
						item.ValidationStatus = "incomplete"
						item.ValidationMsg = "Last sync failed: " + ss.LastError
						v := false
						item.Valid = &v
					} else {
						v := true
						item.Valid = &v
					}
				} else if msg.Result.Status != "" {
					v := false
					item.Valid = &v
				}
				p.flashMessage = item.ValidationMsg
				p.flashMessageAt = time.Now()
				return
			}
		}
	}
}

// validateSlackResult performs a structural check on Slack credentials.
func validateSlackResult() plugin.ValidationResult {
	if config.LoadSlackToken() == "" {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "Slack bot token not configured",
			Hint:    "Press 'a' to enter token or export SLACK_BOT_TOKEN",
		}
	}
	return plugin.ValidationResult{
		Status:  "ok",
		Message: "Slack token configured",
	}
}

// authFlowCmdFunc is a variable for testability — wraps auth.AuthFlowCmd.
var authFlowCmdFunc = func(ctx context.Context, conf *oauth2.Config, tokenPath, clientID, clientSecret string) tea.Cmd {
	return auth.AuthFlowCmd(ctx, auth.AuthFlowOpts{
		Config:       conf,
		TokenPath:    tokenPath,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	})
}

// oauthConfigForSlug returns the OAuth2 config and token path for a data source.
func (p *Plugin) oauthConfigForSlug(slug, clientID, clientSecret string) (*oauth2.Config, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, ""
	}

	switch slug {
	case "calendar":
		return auth.LoadGoogleOAuth2Config(clientID, clientSecret,
			gcal.CalendarScope, gcal.CalendarEventsScope,
		), filepath.Join(home, ".config", "google-calendar-mcp", "credentials.json")
	case "gmail":
		return auth.LoadGoogleOAuth2Config(clientID, clientSecret,
			gm.GmailReadonlyScope,
		), filepath.Join(home, ".gmail-mcp", "work.json")
	}
	return nil, ""
}

// handleAuthFlowResult processes the result of a browser-based OAuth flow.
// Returns a tea.Cmd to trigger an async live recheck of the authenticated
// data source, or nil on failure.
func (p *Plugin) handleAuthFlowResult(msg auth.AuthFlowResultMsg) tea.Cmd {
	p.authCancel = nil
	slug := p.pendingAuthSlug
	p.pendingAuthCreds = nil
	p.pendingAuthSlug = ""

	if msg.Error != nil {
		p.flashMessage = "Auth failed: " + msg.Error.Error()
		p.flashMessageAt = time.Now()
		return nil
	}

	p.flashMessage = "Authenticated! Token saved for " + slug
	p.flashMessageAt = time.Now()

	// Rebuild nav first (structural), then fire an async live recheck so
	// the nav indicator updates to reflect the freshly-saved token.
	p.rebuildNav()
	recheckSlug := slug
	return func() tea.Msg {
		result := p.validateDataSourceResult(recheckSlug, true)
		return datasourceRecheckResult{Slug: recheckSlug, Result: result}
	}
}

// saveSlackToken saves the pending Slack bot token to config, triggers a
// nav rebuild, and returns a tea.Cmd that rechecks credential status.
func (p *Plugin) saveSlackToken() tea.Cmd {
	tok := p.pendingSlackToken
	slug := p.pendingAuthSlug
	p.pendingSlackToken = nil
	p.pendingAuthSlug = ""

	if tok == nil || tok.BotToken == "" {
		p.flashMessage = "No token provided"
		p.flashMessageAt = time.Now()
		return nil
	}

	p.cfg.Slack.BotToken = tok.BotToken
	if err := config.Save(p.cfg); err != nil {
		p.flashMessage = "Failed to save token: " + err.Error()
		p.flashMessageAt = time.Now()
		return nil
	}

	p.flashMessage = "Slack bot token saved"
	p.flashMessageAt = time.Now()
	p.publishConfigSaved("slack")
	p.rebuildNav()

	return func() tea.Msg {
		result := p.validateDataSourceResult(slug, false)
		return datasourceRecheckResult{Slug: slug, Result: result}
	}
}

// cancelAuthFlow cancels any in-progress OAuth flow.
func (p *Plugin) cancelAuthFlow() {
	if p.authCancel != nil {
		p.authCancel()
		p.authCancel = nil
	}
	p.pendingAuthCreds = nil
	p.pendingAuthSlug = ""
}

func renderSwatches(pal config.Palette) string {
	colors := []string{pal.Cyan, pal.Yellow, pal.Purple, pal.Green, pal.White}
	var parts []string
	for _, c := range colors {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// handleBannerKey processes key events for the banner editing content pane.
func (p *Plugin) handleBannerKey(msg tea.KeyMsg) plugin.Action {
	// When editing a text field, route to the textinput.
	if p.bannerEditing {
		switch msg.Type {
		case tea.KeyEnter:
			// Save the value and exit editing.
			p.bannerEditing = false
			p.focusZone = FocusContent
			switch p.bannerField {
			case 0:
				p.bannerNameInput.Blur()
				p.cfg.Name = p.bannerNameInput.Value()
			case 1:
				p.bannerSubtitleInput.Blur()
				p.cfg.Subtitle = p.bannerSubtitleInput.Value()
			case 3:
				p.bannerPaddingInput.Blur()
				if v, err := strconv.Atoi(p.bannerPaddingInput.Value()); err == nil {
					p.cfg.SetBannerTopPadding(v)
				}
				// Sync the input back to the clamped value
				p.bannerPaddingInput.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
			}
			if err := config.Save(p.cfg); err == nil {
				p.flashMessage = "Banner saved"
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
			return plugin.NoopAction()
		case tea.KeyEsc:
			// Cancel editing, restore original value.
			p.bannerEditing = false
			p.focusZone = FocusContent
			switch p.bannerField {
			case 0:
				p.bannerNameInput.SetValue(p.cfg.Name)
				p.bannerNameInput.Blur()
			case 1:
				p.bannerSubtitleInput.SetValue(p.cfg.Subtitle)
				p.bannerSubtitleInput.Blur()
			case 3:
				p.bannerPaddingInput.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
				p.bannerPaddingInput.Blur()
			}
			return plugin.NoopAction()
		default:
			switch p.bannerField {
			case 0:
				p.bannerNameInput, _ = p.bannerNameInput.Update(msg)
			case 1:
				p.bannerSubtitleInput, _ = p.bannerSubtitleInput.Update(msg)
			case 3:
				p.bannerPaddingInput, _ = p.bannerPaddingInput.Update(msg)
			}
			return plugin.NoopAction()
		}
	}

	// Not editing — navigation mode within the banner content pane.
	switch msg.String() {
	case "up", "k":
		if p.bannerField > 0 {
			p.bannerField--
		}
	case "down", "j":
		if p.bannerField < 3 {
			p.bannerField++
		}
	case "enter":
		switch {
		case p.bannerField <= 1:
			// Start editing text field.
			p.bannerEditing = true
			p.focusZone = FocusEditing
			if p.bannerField == 0 {
				p.bannerNameInput.Focus()
			} else {
				p.bannerSubtitleInput.Focus()
			}
		case p.bannerField == 3:
			// Start editing top padding.
			p.bannerEditing = true
			p.focusZone = FocusEditing
			p.bannerPaddingInput.Focus()
		}
	case " ":
		if p.bannerField == 2 {
			// Toggle show/hide.
			p.cfg.SetShowBanner(!p.cfg.BannerVisible())
			if err := config.Save(p.cfg); err == nil {
				if p.cfg.BannerVisible() {
					p.flashMessage = "Banner shown"
				} else {
					p.flashMessage = "Banner hidden"
				}
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
		}
	case "+", "=":
		if p.bannerField == 3 {
			p.cfg.SetBannerTopPadding(p.cfg.GetBannerTopPadding() + 1)
			p.bannerPaddingInput.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
			if err := config.Save(p.cfg); err == nil {
				p.flashMessage = fmt.Sprintf("Top padding: %d", p.cfg.GetBannerTopPadding())
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
		}
	case "-":
		if p.bannerField == 3 {
			p.cfg.SetBannerTopPadding(p.cfg.GetBannerTopPadding() - 1)
			p.bannerPaddingInput.SetValue(fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()))
			if err := config.Save(p.cfg); err == nil {
				p.flashMessage = fmt.Sprintf("Top padding: %d", p.cfg.GetBannerTopPadding())
				p.publishConfigSaved("banner")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			p.flashMessageAt = time.Now()
		}
	}
	return plugin.NoopAction()
}
