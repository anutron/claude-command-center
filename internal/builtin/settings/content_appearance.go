package settings

import (
	"fmt"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Banner content (sidebar layout) ---

func (p *Plugin) viewBannerContent(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("BANNER"))
	lines = append(lines, "")

	fields := []struct {
		label string
		value string
	}{
		{"Name", p.cfg.Name},
		{"Subtitle", p.cfg.Subtitle},
	}

	for i, f := range fields {
		cursor := "  "
		if i == p.bannerField {
			cursor = p.styles.pointer.Render("> ")
		}

		if p.bannerEditing && i == p.bannerField {
			var input string
			if i == 0 {
				input = p.bannerNameInput.View()
			} else {
				input = p.bannerSubtitleInput.View()
			}
			lines = append(lines, fmt.Sprintf("%s%s %s",
				cursor, p.styles.muted.Render(f.label+":"), input))
		} else {
			val := f.value
			if val == "" {
				val = "(empty)"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s",
				cursor, p.styles.muted.Render(f.label+":"), p.styles.itemName.Render(val)))
		}
	}

	// Show/hide toggle
	cursor := "  "
	if p.bannerField == 2 {
		cursor = p.styles.pointer.Render("> ")
	}
	status := p.styles.enabled.Render("[on] ")
	if !p.cfg.BannerVisible() {
		status = p.styles.disabled.Render("[off]")
	}
	lines = append(lines, fmt.Sprintf("%s%s %s",
		cursor, status, p.styles.itemName.Render("Show Banner")))

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  enter edit  space toggle  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handleBannerContentKey(msg tea.KeyMsg) plugin.Action {
	return p.handleBannerKey(msg)
}

// --- Palette content (sidebar layout) ---

func (p *Plugin) viewPaletteContent(width, height int) string {
	var lines []string
	lines = append(lines, p.styles.header.Render("PALETTE"))
	lines = append(lines, "")

	names := config.PaletteNames()
	for i, name := range names {
		pal := config.GetPalette(name, nil)

		cursor := "  "
		if i == p.paletteCursor {
			cursor = p.styles.pointer.Render("> ")
		}

		active := ""
		if name == p.cfg.Palette {
			active = p.styles.enabled.Render(" (active)")
		}

		swatches := renderSwatches(pal)

		nameStyle := p.styles.itemName
		if i == p.paletteCursor {
			nameStyle = nameStyle.Bold(true)
		}

		lines = append(lines, fmt.Sprintf("%s%s%s  %s", cursor, nameStyle.Render(name), active, swatches))
	}

	lines = append(lines, "")
	lines = append(lines, p.styles.muted.Render("  up/down cycle  enter apply  esc back"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *Plugin) handlePaletteContentKey(msg tea.KeyMsg) plugin.Action {
	names := config.PaletteNames()
	switch msg.String() {
	case "up", "k":
		if p.paletteCursor > 0 {
			p.paletteCursor--
		}
	case "down", "j":
		if p.paletteCursor < len(names)-1 {
			p.paletteCursor++
		}
	case "enter":
		selected := names[p.paletteCursor]
		previous := p.cfg.Palette
		p.cfg.Palette = selected
		if err := config.Save(p.cfg); err == nil {
			// Rebuild all styles so the palette change is visible immediately.
			newPal := config.GetPalette(selected, p.cfg.Colors)
			p.styles = newSettingsStyles(newPal)
			if p.sharedStyles != nil {
				*p.sharedStyles = ui.NewStyles(newPal)
			}
			if p.sharedGrad != nil {
				*p.sharedGrad = ui.NewGradientColors(newPal)
			}

			p.flashMessage = "Palette saved: " + selected
			p.publishConfigSaved("palette")
			if p.bus != nil {
				p.bus.Publish(plugin.Event{
					Source: "settings",
					Topic:  "palette.changed",
					Payload: map[string]interface{}{
						"previous": previous,
						"new":      selected,
					},
				})
			}
		} else {
			p.flashMessage = "Failed to save palette: " + err.Error()
		}
		p.flashMessageAt = time.Now()
	}
	return plugin.NoopAction()
}
