package settings

import (
	"fmt"
	"strings"

	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/charmbracelet/huh"
	tea "github.com/charmbracelet/bubbletea"
)

// pluginFormValues holds the selected action from a plugin pane form.
type pluginFormValues struct {
	Action string
}

// --- Plugin form ---

func (p *Plugin) buildPluginForm(item *NavItem) *huh.Form {
	p.pluginValues = &pluginFormValues{}

	// Build note content with plugin info
	var noteLines []string

	// Check if the plugin implements SettingsProvider (from registry)
	var hasProvider bool
	if plug, ok := p.registry.BySlug(item.Slug); ok {
		if sp, ok := plug.(plugin.SettingsProvider); ok {
			hasProvider = true
			noteLines = append(noteLines, sp.SettingsView(80, 20))
		}
	}

	// Check providers map (data source style providers on plugins)
	if sp, ok := p.providers[item.Slug]; ok && !hasProvider {
		hasProvider = true
		noteLines = append(noteLines, sp.SettingsView(80, 20))
	}

	if !hasProvider {
		// Default plugin info
		noteLines = append(noteLines, fmt.Sprintf("%s %s",
			p.styles.muted.Render("Slug:"),
			p.styles.itemName.Render(item.Slug)))
		noteLines = append(noteLines, fmt.Sprintf("%s %s",
			p.styles.muted.Render("Type:"),
			p.styles.itemName.Render(item.Kind)))

		if item.Enabled != nil {
			statusText := "Disabled"
			statusStyle := p.styles.disabled
			if *item.Enabled {
				statusText = "Enabled"
				statusStyle = p.styles.enabled
			}
			noteLines = append(noteLines, fmt.Sprintf("%s %s",
				p.styles.muted.Render("Status:"),
				statusStyle.Render(statusText)))
		}

		// External plugin command info
		if strings.HasPrefix(item.Slug, "external-") {
			for i, ep := range p.cfg.ExternalPlugins {
				if item.Slug == fmt.Sprintf("external-%d", i) {
					noteLines = append(noteLines, fmt.Sprintf("%s %s",
						p.styles.muted.Render("Command:"),
						p.styles.itemName.Render(ep.Command)))
					break
				}
			}
		}
	}

	vals := p.pluginValues

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(strings.ToUpper(item.Label)).
				DescriptionFunc(func() string {
					return strings.Join(noteLines, "\n")
				}, &vals.Action),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

func (p *Plugin) handlePluginFormCompletion(slug string) tea.Cmd {
	p.pluginValues = nil

	// Rebuild the form so it stays on screen
	if item := p.findNavItem(slug); item != nil {
		form := p.buildPluginForm(item)
		if form != nil {
			p.activeForm = form
			p.activeFormSlug = slug
			return form.Init()
		}
	}

	return nil
}
