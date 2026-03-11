package settings

import (
	"fmt"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FocusZone indicates which panel has keyboard focus.
type FocusZone int

const (
	FocusNav FocusZone = iota
	FocusContent
	FocusEditing
)

// Category groups NavItems under a heading.
type Category struct {
	Label string
	Items []NavItem
}

// NavItem represents a single entry in the sidebar navigation.
type NavItem struct {
	Label      string
	Slug       string // unique: "banner", "palette", "calendar", "system-schedule", etc.
	Kind       string // "appearance", "plugin", "datasource", "system"
	Enabled    *bool  // nil = no toggle, non-nil = on/off
	Toggleable bool
	Valid      *bool
	ValidHint  string
}

// rebuildNav populates the sidebar categories from config and registry.
func (p *Plugin) rebuildNav() {
	p.navCategories = nil

	// --- APPEARANCE ---
	appearance := Category{
		Label: "APPEARANCE",
		Items: []NavItem{
			{Label: "Banner", Slug: "banner", Kind: "appearance"},
			{Label: "Palette", Slug: "palette", Kind: "appearance"},
		},
	}
	p.navCategories = append(p.navCategories, appearance)

	// --- PLUGINS ---
	var pluginItems []NavItem
	if p.registry != nil {
		for _, plug := range p.registry.All() {
			slug := plug.Slug()
			// Settings itself is not shown as toggleable
			if slug == "settings" {
				continue
			}
			enabled := p.cfg.PluginEnabled(slug)
			pluginItems = append(pluginItems, NavItem{
				Label:      plug.TabName(),
				Slug:       slug,
				Kind:       "plugin",
				Enabled:    &enabled,
				Toggleable: true,
			})
		}
	}
	// Threads data source — shown in PLUGINS as a toggleable item
	threadsEnabled := p.cfg.Threads.Enabled
	pluginItems = append(pluginItems, NavItem{
		Label:      "Threads",
		Slug:       "threads",
		Kind:       "plugin",
		Enabled:    &threadsEnabled,
		Toggleable: true,
	})

	// External plugins
	for i, ep := range p.cfg.ExternalPlugins {
		enabled := ep.Enabled
		pluginItems = append(pluginItems, NavItem{
			Label:      ep.Name,
			Slug:       fmt.Sprintf("external-%d", i),
			Kind:       "plugin",
			Enabled:    &enabled,
			Toggleable: true,
		})
	}
	if len(pluginItems) > 0 {
		p.navCategories = append(p.navCategories, Category{
			Label: "PLUGINS",
			Items: pluginItems,
		})
	}

	// --- DATA SOURCES ---
	type dsEntry struct {
		label   string
		slug    string
		enabled bool
		toggle  bool
	}
	dataSources := []dsEntry{
		{"Calendar", "calendar", p.cfg.Calendar.Enabled, true},
		{"GitHub", "github", p.cfg.GitHub.Enabled, true},
		{"Granola", "granola", p.cfg.Granola.Enabled, true},
		{"Slack", "slack", p.cfg.Slack.Enabled, true},
		{"Gmail", "gmail", p.cfg.Gmail.Enabled, true},
	}
	var dsItems []NavItem
	for _, ds := range dataSources {
		enabled := ds.enabled
		item := NavItem{
			Label:      ds.label,
			Slug:       ds.slug,
			Kind:       "datasource",
			Enabled:    &enabled,
			Toggleable: ds.toggle,
		}
		// Validate credentials
		if err := p.validateDataSource(ds.slug); err != nil {
			v := false
			item.Valid = &v
			item.ValidHint = err.Error()
		} else {
			v := true
			item.Valid = &v
		}
		dsItems = append(dsItems, item)
	}
	p.navCategories = append(p.navCategories, Category{
		Label: "DATA SOURCES",
		Items: dsItems,
	})

	// --- SYSTEM ---
	system := Category{
		Label: "SYSTEM",
		Items: []NavItem{
			{Label: "Schedule", Slug: "system-schedule", Kind: "system"},
			{Label: "MCP Servers", Slug: "system-mcp", Kind: "system"},
			{Label: "Skills", Slug: "system-skills", Kind: "system"},
			{Label: "Shell Integration", Slug: "system-shell", Kind: "system"},
			{Label: "Logs", Slug: "system-logs", Kind: "system"},
		},
	}
	p.navCategories = append(p.navCategories, system)
}

// navItemCount returns the total number of selectable items (excluding category headers).
func (p *Plugin) navItemCount() int {
	n := 0
	for _, cat := range p.navCategories {
		n += len(cat.Items)
	}
	return n
}

// selectedNavItem returns the NavItem at the current nav cursor position.
// Returns nil if the cursor is out of range.
func (p *Plugin) selectedNavItem() *NavItem {
	idx := 0
	for i := range p.navCategories {
		for j := range p.navCategories[i].Items {
			if idx == p.navCursor {
				return &p.navCategories[i].Items[j]
			}
			idx++
		}
	}
	return nil
}

// navCursorUp moves the nav cursor up by one, clamping at 0.
func (p *Plugin) navCursorUp() {
	if p.navCursor > 0 {
		p.navCursor--
	}
}

// navCursorDown moves the nav cursor down by one, clamping at the last item.
func (p *Plugin) navCursorDown() {
	max := p.navItemCount() - 1
	if max < 0 {
		max = 0
	}
	if p.navCursor < max {
		p.navCursor++
	}
}

// viewSidebar renders the sidebar navigation panel.
func (p *Plugin) viewSidebar(width, height int, focus FocusZone) string {
	var lines []string

	itemIdx := 0
	for _, cat := range p.navCategories {
		// Category header
		lines = append(lines, p.styles.categoryHeader.Render(" "+cat.Label))

		for _, item := range cat.Items {
			selected := itemIdx == p.navCursor

			// Cursor indicator
			cursor := "  "
			if selected && focus == FocusNav {
				cursor = p.styles.pointer.Render("> ")
			} else if selected {
				cursor = "* "
			}

			// Toggle indicator
			toggle := ""
			if item.Toggleable && item.Enabled != nil {
				if *item.Enabled {
					toggle = p.styles.navEnabled.Render("[on] ")
				} else {
					toggle = p.styles.navDisabled.Render("[off]")
				}
			}

			// Validation indicator
			valid := ""
			if item.Valid != nil {
				if *item.Valid {
					valid = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render("\u2713")
				} else {
					valid = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render("\u2717")
				}
			}

			// Label styling
			var label string
			if selected && focus == FocusNav {
				label = p.styles.navSelected.Render(item.Label)
			} else if item.Enabled != nil && !*item.Enabled {
				label = p.styles.navDisabled.Render(item.Label)
			} else {
				label = p.styles.navUnselected.Render(item.Label)
			}

			if toggle != "" {
				lines = append(lines, fmt.Sprintf("%s%s %s%s", cursor, toggle, label, valid))
			} else {
				lines = append(lines, fmt.Sprintf("%s%s%s", cursor, label, valid))
			}
			itemIdx++
		}

		// Blank line between categories
		lines = append(lines, "")
	}

	// Remove trailing blank line
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	content := strings.Join(lines, "\n")

	// Apply sidebar panel style based on focus
	var panelStyle lipgloss.Style
	if focus == FocusNav {
		panelStyle = p.styles.sidebarFocused
	} else {
		panelStyle = p.styles.sidebarUnfocused
	}

	return panelStyle.Width(width).Height(height).Render(content)
}

// handleNavKey processes key events when the nav sidebar is focused.
// Returns a plugin.Action indicating what happened.
func (p *Plugin) handleNavKey(msg tea.KeyMsg) plugin.Action {
	switch msg.String() {
	case "up", "k":
		p.navCursorUp()
	case "down", "j":
		p.navCursorDown()
	case " ":
		// Toggle if item is toggleable
		item := p.selectedNavItem()
		if item == nil || !item.Toggleable {
			return plugin.NoopAction()
		}
		if item.Enabled != nil {
			newVal := !*item.Enabled
			item.Enabled = &newVal
			p.applyNavToggle(item)
		}
	case "enter", "right", "l":
		p.focusZone = FocusContent
	case "esc":
		return plugin.Action{Type: plugin.ActionUnhandled}
	}
	return plugin.NoopAction()
}

// applyNavToggle persists a toggle change from a NavItem to config.
func (p *Plugin) applyNavToggle(item *NavItem) {
	if item.Enabled == nil {
		return
	}
	enabled := *item.Enabled

	switch item.Kind {
	case "plugin":
		// Threads data source — uses ThreadsConfig.Enabled
		if item.Slug == "threads" {
			p.cfg.Threads.Enabled = enabled
			if err := config.Save(p.cfg); err == nil {
				if enabled {
					p.flashMessage = "Threads enabled"
				} else {
					p.flashMessage = "Threads disabled"
				}
				p.publishConfigSaved("threads")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
			break
		}
		// Check if it's an external plugin
		if strings.HasPrefix(item.Slug, "external-") {
			epIdx := -1
			for i := range p.cfg.ExternalPlugins {
				if item.Slug == fmt.Sprintf("external-%d", i) {
					epIdx = i
					break
				}
			}
			if epIdx < 0 {
				return
			}
			p.cfg.ExternalPlugins[epIdx].Enabled = enabled
			if err := config.Save(p.cfg); err == nil {
				p.flashMessage = "Restart CCC to apply"
				p.publishConfigSaved("external_plugins")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
		} else {
			// Built-in plugin
			p.cfg.SetPluginEnabled(item.Slug, enabled)
			if err := config.Save(p.cfg); err == nil {
				if enabled {
					p.flashMessage = item.Label + " enabled"
				} else {
					p.flashMessage = item.Label + " disabled"
				}
				p.publishConfigSaved("disabled_plugins")
			} else {
				p.flashMessage = "Failed to save: " + err.Error()
			}
		}

	case "datasource":
		// Validate credentials when enabling
		if enabled {
			if err := p.validateDataSource(item.Slug); err != nil {
				v := false
				item.Enabled = &v
				p.flashMessage = err.Error()
				p.flashMessageAt = currentTime()
				return
			}
		}
		switch item.Slug {
		case "calendar":
			p.cfg.Calendar.Enabled = enabled
		case "github":
			p.cfg.GitHub.Enabled = enabled
		case "granola":
			p.cfg.Granola.Enabled = enabled
		case "slack":
			p.cfg.Slack.Enabled = enabled
		case "gmail":
			p.cfg.Gmail.Enabled = enabled
		}
		if err := config.Save(p.cfg); err == nil {
			p.flashMessage = "Changes apply on next refresh"
			p.publishConfigSaved(item.Slug)
			if p.bus != nil {
				p.bus.Publish(plugin.Event{
					Source: "settings",
					Topic:  "datasource.toggled",
					Payload: map[string]interface{}{
						"name":    item.Slug,
						"enabled": enabled,
					},
				})
			}
		} else {
			p.flashMessage = "Failed to save: " + err.Error()
		}
	}
	p.flashMessageAt = currentTime()
}

// currentTime is a helper for testability — returns time.Now().
func currentTime() time.Time {
	return time.Now()
}
