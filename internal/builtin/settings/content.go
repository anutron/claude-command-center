package settings

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// viewContent renders the content pane based on the currently selected nav item.
func (p *Plugin) viewContent(width, height int) string {
	selected := p.selectedNavItem()

	// Check if the selected nav item is the logs pane — render logs content
	// even from FocusNav so scrolling via forwarded keys is visible.
	isLogs := selected != nil && selected.Slug == "system-logs"
	isAutomations := selected != nil && selected.Slug == "system-automations"

	// Build header from the currently selected nav item.
	var header string
	if selected != nil {
		headerLines := p.renderPaneHeader(strings.ToUpper(selected.Label), selected.Description)
		header = strings.Join(headerLines, "\n")
	}

	var body string
	switch {
	case p.activeForm != nil && (selected == nil || p.activeFormSlug == selected.Slug):
		body = p.viewActiveFormContent(width, height)
	case isAutomations:
		body = p.viewAutomationsContent(width, height)
	case p.focusZone == FocusLogs || isLogs:
		body = p.viewLogsContent(width, height)
	default:
		// When navigating the sidebar, show a read-only preview of the
		// currently highlighted item's form content. The form is built
		// transiently — it is NOT stored as p.activeForm so it won't
		// receive key events or Init commands (BUG-046).
		if item := p.selectedNavItem(); item != nil {
			body = p.viewPreviewContent(item, width, height)
		} else {
			body = p.styles.muted.Render("  Select an item from the sidebar")
		}
	}

	// Prepend header to body for all pane types.
	if header != "" {
		body = header + "\n" + body
	}

	// Pick panel style based on focus zone.
	var panelStyle lipgloss.Style
	if p.focusZone == FocusForm || p.focusZone == FocusLogs {
		panelStyle = p.styles.contentFocused
	} else {
		panelStyle = p.styles.contentUnfocused
	}

	return panelStyle.Width(width).Height(height).Render(body)
}

// viewActiveFormContent renders the content pane when a huh form is active.
// For datasources with a SettingsProvider, the provider's interactive view is
// rendered above the form so it remains interactive (BUG-050).
func (p *Plugin) viewActiveFormContent(width, height int) string {
	item := p.selectedNavItem()
	if item != nil && item.Kind == "datasource" {
		if sp, ok := p.providers[item.Slug]; ok {
			providerView := sp.SettingsView(width, height)
			return providerView + "\n\n" + p.activeForm.View()
		}
	}
	return p.activeForm.View()
}

// viewPreviewContent renders a read-only preview of the currently highlighted
// nav item. For datasources with providers, the provider view is shown above
// the transient form preview.
func (p *Plugin) viewPreviewContent(item *NavItem, width, height int) string {
	form, initCmd := p.buildFormForSlug(item)
	if form == nil {
		return p.styles.muted.Render("  Select an item from the sidebar")
	}

	// Process Init commands so the form's internal state (focus, field
	// rendering) is fully set up. Without this, Select and Note fields
	// with DescriptionFunc don't render their content (BUG-062).
	if initCmd != nil {
		if msg := initCmd(); msg != nil {
			form.Update(msg)
		}
	}

	if item.Kind == "datasource" {
		if sp, ok := p.providers[item.Slug]; ok {
			providerView := sp.SettingsView(width, height)
			return providerView + "\n\n" + form.View()
		}
	}
	return form.View()
}

// renderPaneHeader renders a styled header title with an optional dimmed description line below it.
func (p *Plugin) renderPaneHeader(title, description string) []string {
	lines := []string{p.styles.header.Render(title)}
	if description != "" {
		lines = append(lines, "  "+p.styles.muted.Render(description))
	}
	lines = append(lines, "")
	return lines
}
