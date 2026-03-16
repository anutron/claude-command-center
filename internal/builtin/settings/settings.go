// Package settings implements the Settings plugin for CCC.
// It provides a sidebar-based UI for appearance, plugins, data sources,
// system actions, and logs.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	navCategories   []Category
	navCursor       int
	navScrollOffset int // scroll offset for sidebar when items exceed panel height
	focusZone       FocusZone

	// Banner form values (bound to active huh form)
	bannerValues *bannerFormValues

	// Palette form values (bound to active huh form)
	paletteValues *paletteFormValues

	// System form values (bound to active huh form for system panes)
	systemValues *systemFormValues

	// Datasource form values (bound to active huh form for datasource panes)
	datasourceValues *datasourceFormValues

	// Plugin form values (bound to active huh form for plugin panes)
	pluginValues *pluginFormValues

	// Logs state (used by content_logs)
	logOffset      int
	logFilterInput textinput.Model
	logFilterMode  bool // true when filter input is focused

	// Active huh form (nil when no form is displayed)
	activeForm     *huh.Form
	activeFormSlug string // slug of the nav item the form belongs to

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

	// Initialize providers map and register data source settings providers.
	if p.providers == nil {
		p.providers = make(map[string]plugin.SettingsProvider)
	}
	p.providers["calendar"] = calendar.NewSettings(p.cfg, pal, p.logger)
	p.providers["github"] = ghsettings.NewSettings(p.cfg, pal, p.logger)
	p.providers["granola"] = granola.NewSettings(p.cfg, pal)

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
	case FocusForm:
		// If a huh form is active, route keys to the form handler.
		if p.activeForm != nil {
			return p.handleFormKey(msg)
		}
		// No active form — esc/left/h returns to nav.
		switch msg.String() {
		case "esc", "left", "h":
			p.focusZone = FocusNav
		}
		return plugin.NoopAction()
	case FocusLogs:
		return p.handleLogsContentKey(msg)
	}
	return plugin.NoopAction()
}

// handleFormKey processes key events when a huh form is focused.
func (p *Plugin) handleFormKey(msg tea.KeyMsg) plugin.Action {
	if p.activeForm == nil {
		p.focusZone = FocusForm
		return plugin.NoopAction()
	}

	// Left arrow navigates back to the nav sidebar, unless the focused field
	// is a text input (huh.Input) where left arrow moves the cursor.
	if msg.String() == "left" {
		if _, isInput := p.activeForm.GetFocusedField().(*huh.Input); !isInput {
			slug := p.activeFormSlug
			if shouldAutoSaveOnExit(slug) {
				p.handleFormCompletion(slug)
			}
			p.focusZone = FocusNav
			return plugin.NoopAction()
		}
	}

	// Allow escape to cancel the form and any pending auth.
	// For datasource providers, esc returns to nav only if the provider
	// doesn't consume it (e.g., color picker or fetch mode use esc internally).
	if msg.String() == "esc" {
		// Let the provider handle esc first (color picker cancel, fetch mode exit, etc.)
		if sp, ok := p.providers[p.activeFormSlug]; ok {
			action := sp.HandleSettingsKey(msg)
			if action.Type != plugin.ActionUnhandled {
				return p.wrapProviderAction(action)
			}
		}

		slug := p.activeFormSlug

		// Auto-save editable form values (banner, palette) before dismissing.
		// Auth and action forms are NOT auto-saved — esc means cancel.
		if shouldAutoSaveOnExit(slug) {
			p.handleFormCompletion(slug)
		}

		p.activeForm = nil
		p.activeFormSlug = ""
		p.cancelAuthFlow()
		p.pendingAuthCreds = nil
		p.pendingSlackToken = nil
		p.pendingAuthSlug = ""
		// Return to nav if the form was the primary content (banner, palette);
		// otherwise stay in FocusForm so the content pane remains visible.
		if isFormOnlySlug(slug) {
			p.focusZone = FocusNav
		} else {
			p.focusZone = FocusForm
		}
		return plugin.NoopAction()
	}

	// For datasources with interactive providers (calendar list, GitHub repos),
	// offer the key to the provider first. If the provider handles it, consume
	// the key without forwarding to the huh form (BUG-050).
	if sp, ok := p.providers[p.activeFormSlug]; ok {
		action := sp.HandleSettingsKey(msg)
		if action.Type != plugin.ActionUnhandled {
			return p.wrapProviderAction(action)
		}
	}

	// Forward key to the form
	form, cmd := p.activeForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		p.activeForm = f
	}

	// Auto-save on field transition keys (tab, enter) for editable forms.
	// The bound value pointers are updated by huh in real-time, so we can
	// read them immediately after Update() and persist without rebuilding.
	if p.activeForm != nil && p.activeForm.State != huh.StateCompleted {
		key := msg.String()
		if (key == "tab" || key == "shift+tab" || key == "enter") && shouldAutoSaveOnExit(p.activeFormSlug) {
			p.saveFormValues(p.activeFormSlug)
		}
	}

	// Check if form completed
	if p.activeForm.State == huh.StateCompleted {
		slug := p.activeFormSlug
		p.activeForm = nil
		p.activeFormSlug = ""

		// Run the completion handler for this slug.
		completionCmd := p.handleFormCompletion(slug)
		if completionCmd != nil {
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmd, completionCmd)}
		}
		if cmd != nil {
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return plugin.NoopAction()
	}

	if cmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
	}
	// Form is active but the key produced no cmd (e.g. Tab on a single-field
	// form). Return a consumed action so the TUI host doesn't switch tabs
	// while a form is visible (BUG-041).
	return plugin.ConsumedAction()
}

// wrapProviderAction converts a provider's Action into a host-compatible Action,
// translating flash messages and tea.Cmds appropriately.
func (p *Plugin) wrapProviderAction(action plugin.Action) plugin.Action {
	if action.Type == plugin.ActionFlash {
		p.flashMessage = action.Payload
		p.flashMessageAt = time.Now()
		if action.TeaCmd != nil {
			return plugin.Action{Type: plugin.ActionNoop, TeaCmd: action.TeaCmd}
		}
		return plugin.NoopAction()
	}
	if action.TeaCmd != nil {
		return plugin.Action{Type: plugin.ActionNoop, TeaCmd: action.TeaCmd}
	}
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
		handled, cmd := p.handleSystemActionResult(msg)
		if cmd != nil {
			return handled, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return handled, plugin.NoopAction()
	case auth.AuthFlowResultMsg:
		cmd := p.handleAuthFlowResult(msg)
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
	case plugin.TabLeaveMsg:
		// Auto-save editable form values before leaving tab
		if p.activeForm != nil {
			if shouldAutoSaveOnExit(p.activeFormSlug) {
				p.handleFormCompletion(p.activeFormSlug)
			}
			p.activeForm = nil
			p.activeFormSlug = ""
			p.focusZone = FocusForm
		}
		// Cancel any in-progress OAuth flow and clear pending token state
		p.cancelAuthFlow()
		p.pendingSlackToken = nil
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

	// Route non-key messages to the active form when a form is displayed.
	// This runs AFTER provider routing so that async fetch results
	// (CalendarFetchResultMsg, ghRepoFetchResult) reach the provider even
	// when a huh form is active (BUG-064, BUG-065).
	if p.activeForm != nil {
		form, cmd := p.activeForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			p.activeForm = f
		}
		if p.activeForm.State == huh.StateCompleted {
			slug := p.activeFormSlug
			p.activeForm = nil
			p.activeFormSlug = ""

			completionCmd := p.handleFormCompletion(slug)
			if completionCmd != nil {
				return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: tea.Batch(cmd, completionCmd)}
			}
		}
		if cmd != nil {
			return true, plugin.Action{Type: plugin.ActionNoop, TeaCmd: cmd}
		}
		return true, plugin.NoopAction()
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
	case FocusForm:
		if p.activeForm != nil {
			help = p.styles.muted.Render("  tab next field  enter submit  left/esc save & back")
		} else {
			help = p.styles.muted.Render("  esc/left sidebar")
		}
	case FocusLogs:
		help = p.styles.muted.Render("  j/k scroll  f/b page  d/u half-page  / filter  esc back")
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
		if live {
			return liveSlackTokenCheck()
		}
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

				// Apply sync-aware downgrade for structural (non-live) checks:
				// credentials may look "ok" structurally but if sync has
				// never succeeded or last sync failed, downgrade to
				// "incomplete" with a warning indicator (BUG-030).
				//
				// When a LIVE check returns "ok", the API actually responded
				// successfully — stale DB sync errors are irrelevant (BUG-053).
				if msg.Result.Status == "ok" && msg.Live {
					// Live "ok" is authoritative — skip sync-aware downgrade.
					v := true
					item.Valid = &v
				} else if msg.Result.Status == "ok" {
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
			Message: "Slack token not configured",
			Hint:    "Press 'a' to enter token or export SLACK_TOKEN",
		}
	}
	return plugin.ValidationResult{
		Status:  "ok",
		Message: "Slack token configured",
	}
}

// liveSlackTokenCheck calls Slack's auth.test API to verify the token is valid.
func liveSlackTokenCheck() plugin.ValidationResult {
	token := config.LoadSlackToken()
	if token == "" {
		return plugin.ValidationResult{
			Status:  "missing",
			Message: "Slack token not configured",
			Hint:    "Press 'a' to enter token or export SLACK_TOKEN",
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", "https://slack.com/api/auth.test", nil)
	if err != nil {
		return plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Failed to create request",
			Hint:    err.Error(),
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Cannot reach Slack API",
			Hint:    err.Error(),
		}
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  string `json:"user"`
		Team  string `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Invalid response from Slack API",
			Hint:    err.Error(),
		}
	}

	if !result.OK {
		return plugin.ValidationResult{
			Status:  "incomplete",
			Message: "Slack token invalid: " + result.Error,
			Hint:    "Enter a new token or check permissions",
		}
	}

	return plugin.ValidationResult{
		Status:  "ok",
		Message: fmt.Sprintf("Slack token valid (%s @ %s)", result.User, result.Team),
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

	// Rebuild the datasource form so the pane stays populated after
	// OAuth completes, instead of falling to title-only preview (BUG-066).
	var initCmd tea.Cmd
	if item := p.findNavItem(slug); item != nil {
		form := p.buildDatasourceForm(item)
		p.activeForm = form
		p.activeFormSlug = slug
		initCmd = form.Init()
	}

	recheckSlug := slug
	recheckCmd := func() tea.Msg {
		result := p.validateDataSourceResult(recheckSlug, true)
		return datasourceRecheckResult{Slug: recheckSlug, Result: result, Live: true}
	}

	if initCmd != nil {
		return tea.Batch(initCmd, recheckCmd)
	}
	return recheckCmd
}

// saveSlackToken saves the pending Slack token to config, triggers a
// nav rebuild, and returns a tea.Cmd that rechecks credential status.
func (p *Plugin) saveSlackToken() tea.Cmd {
	tok := p.pendingSlackToken
	slug := p.pendingAuthSlug
	p.pendingSlackToken = nil
	p.pendingAuthSlug = ""

	if tok == nil || tok.Token == "" {
		p.flashMessage = "No token provided"
		p.flashMessageAt = time.Now()
		return nil
	}

	p.cfg.Slack.Token = strings.TrimSpace(tok.Token)
	p.cfg.Slack.Enabled = true
	if err := config.Save(p.cfg, true); err != nil {
		p.flashMessage = "Failed to save token: " + err.Error()
		p.flashMessageAt = time.Now()
		return nil
	}

	p.flashMessage = "Slack token saved"
	p.flashMessageAt = time.Now()
	p.publishConfigSaved("slack")
	p.rebuildNav()

	return func() tea.Msg {
		result := p.validateDataSourceResult(slug, true)
		return datasourceRecheckResult{Slug: slug, Result: result, Live: true}
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

// findNavItem returns the NavItem with the given slug, or nil if not found.
func (p *Plugin) findNavItem(slug string) *NavItem {
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			if p.navCategories[i].Items[j].Slug == slug {
				return &p.navCategories[i].Items[j]
			}
		}
	}
	return nil
}

// isFormOnlySlug returns true for slugs where the huh form is the entire
// content pane (no underlying custom view). Pressing Esc on these forms
// should return to nav rather than staying in FocusForm.
func isFormOnlySlug(slug string) bool {
	switch slug {
	case "banner", "palette",
		"system-schedule", "system-mcp", "system-skills", "system-shell":
		return true
	}
	return false
}

// shouldAutoSaveOnExit returns true for slugs whose form values should be
// persisted when the user exits the form (esc, left/h, tab-leave) without
// completing it. This applies to forms with editable settings (banner,
// palette) but NOT to action-based forms (system, datasource, plugin) or
// auth credential forms.
func shouldAutoSaveOnExit(slug string) bool {
	switch slug {
	case "banner", "palette":
		return true
	}
	return false
}

// buildFormForSlug creates a huh.Form for the given nav item and returns it
// along with an init command. Returns (nil, nil) if the slug does not have a
// form-based UI yet — the content pane will use its existing view/key handlers.
func (p *Plugin) buildFormForSlug(item *NavItem) (*huh.Form, tea.Cmd) {
	switch item.Slug {
	case "banner":
		form := p.buildBannerForm()
		return form, form.Init()
	case "palette":
		form := p.buildPaletteForm()
		return form, form.Init()
	case "system-schedule":
		form := p.buildScheduleForm()
		return form, form.Init()
	case "system-mcp":
		form := p.buildMCPForm()
		return form, form.Init()
	case "system-skills":
		form := p.buildSkillsForm()
		return form, form.Init()
	case "system-shell":
		form := p.buildShellForm()
		return form, form.Init()
	case "system-logs":
		return nil, nil
	default:
		// Plugins and data sources
		switch item.Kind {
		case "datasource":
			form := p.buildDatasourceForm(item)
			return form, form.Init()
		case "plugin":
			form := p.buildPluginForm(item)
			if form != nil {
				return form, form.Init()
			}
		}
		return nil, nil
	}
}

// saveFormValues persists the current bound form values for auto-saveable
// slugs WITHOUT rebuilding the form or clearing the value pointers. This is
// used when the user tabs between fields so changes are saved incrementally
// while keeping the form intact and focus undisturbed.
func (p *Plugin) saveFormValues(slug string) {
	switch slug {
	case "banner":
		p.saveBannerValues()
	case "palette":
		p.savePaletteValues()
	}
}

// handleFormCompletion is called when a huh.Form reaches StateCompleted.
// It reads form values, saves config, publishes events, and optionally
// returns a tea.Cmd for async follow-up work.
func (p *Plugin) handleFormCompletion(slug string) tea.Cmd {
	switch slug {
	case "banner":
		return p.handleBannerFormCompletion()
	case "palette":
		return p.handlePaletteFormCompletion()
	case "system-schedule":
		return p.handleScheduleFormCompletion()
	case "system-mcp":
		return p.handleMCPFormCompletion()
	case "system-skills":
		return p.handleSkillsFormCompletion()
	case "system-shell":
		return p.handleShellFormCompletion()
	// Auth-related form completions (pre-existing)
	case "calendar", "gmail":
		// If this was a credential form for OAuth, chain to the auth flow.
		if p.pendingAuthCreds != nil && p.pendingAuthSlug != "" {
			authCmd := p.startAuthFlowForDatasource()
			// Rebuild the datasource form so the pane stays populated
			// while the OAuth flow runs in the background (BUG-066).
			if item := p.findNavItem(slug); item != nil {
				form := p.buildDatasourceForm(item)
				p.activeForm = form
				p.activeFormSlug = slug
				initCmd := form.Init()
				if authCmd != nil {
					return tea.Batch(initCmd, authCmd)
				}
				return initCmd
			}
			return authCmd
		}
		return p.handleDatasourceFormCompletion(slug)
	case "slack":
		// If this was a Slack token form, save and recheck.
		if p.pendingSlackToken != nil && p.pendingAuthSlug == "slack" {
			recheckCmd := p.saveSlackToken()
			// Rebuild the datasource form so the pane stays populated
			// instead of falling back to a title-only preview (BUG-066).
			if item := p.findNavItem(slug); item != nil {
				form := p.buildDatasourceForm(item)
				p.activeForm = form
				p.activeFormSlug = slug
				initCmd := form.Init()
				if recheckCmd != nil {
					return tea.Batch(initCmd, recheckCmd)
				}
				return initCmd
			}
			return recheckCmd
		}
		return p.handleDatasourceFormCompletion(slug)
	default:
		// Check if this is a datasource or plugin slug
		if item := p.findNavItem(slug); item != nil {
			switch item.Kind {
			case "datasource":
				return p.handleDatasourceFormCompletion(slug)
			case "plugin":
				return p.handlePluginFormCompletion(slug)
			}
		}
		return nil
	}
}


