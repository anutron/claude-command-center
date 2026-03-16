package settings

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// --- Banner form values ---

// bannerFormValues holds values bound to the banner huh form fields.
type bannerFormValues struct {
	Name     string
	Subtitle string
	Show     bool
	Padding  string
}

// buildBannerForm creates a huh form for editing banner settings.
func (p *Plugin) buildBannerForm() *huh.Form {
	p.bannerValues = &bannerFormValues{
		Name:     p.cfg.Name,
		Subtitle: p.cfg.Subtitle,
		Show:     p.cfg.BannerVisible(),
		Padding:  fmt.Sprintf("%d", p.cfg.GetBannerTopPadding()),
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				CharLimit(20).
				Value(&p.bannerValues.Name),
			huh.NewInput().
				Title("Subtitle").
				CharLimit(30).
				Value(&p.bannerValues.Subtitle),
			huh.NewConfirm().
				Title("Show Banner").
				Value(&p.bannerValues.Show),
			huh.NewInput().
				Title("Top Padding").
				CharLimit(2).
				Value(&p.bannerValues.Padding).
				Validate(func(s string) error {
					v, err := strconv.Atoi(s)
					if err != nil {
						return errors.New("must be a number")
					}
					if v < 0 || v > 10 {
						return errors.New("must be 0-10")
					}
					return nil
				}),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

// saveBannerValues persists the current banner form values to config without
// rebuilding the form. Used for incremental auto-save on field transitions.
func (p *Plugin) saveBannerValues() {
	if p.bannerValues == nil {
		return
	}

	p.cfg.Name = p.bannerValues.Name
	p.cfg.Subtitle = p.bannerValues.Subtitle
	p.cfg.SetShowBanner(p.bannerValues.Show)

	if v, err := strconv.Atoi(p.bannerValues.Padding); err == nil {
		p.cfg.SetBannerTopPadding(v)
	}

	if err := config.Save(p.cfg, true); err == nil {
		p.flashMessage = "Banner saved"
		p.publishConfigSaved("banner")
	} else {
		p.flashMessage = "Failed to save: " + err.Error()
	}
	p.flashMessageAt = time.Now()
}

// handleBannerFormCompletion saves banner form values to config and publishes events.
func (p *Plugin) handleBannerFormCompletion() tea.Cmd {
	if p.bannerValues == nil {
		return nil
	}

	vals := p.bannerValues
	p.bannerValues = nil

	p.cfg.Name = vals.Name
	p.cfg.Subtitle = vals.Subtitle
	p.cfg.SetShowBanner(vals.Show)

	if v, err := strconv.Atoi(vals.Padding); err == nil {
		p.cfg.SetBannerTopPadding(v)
	}

	if err := config.Save(p.cfg, true); err == nil {
		p.flashMessage = "Banner saved"
		p.publishConfigSaved("banner")
	} else {
		p.flashMessage = "Failed to save: " + err.Error()
	}
	p.flashMessageAt = time.Now()

	// Rebuild the form so it stays on screen with updated values
	form := p.buildBannerForm()
	p.activeForm = form
	p.activeFormSlug = "banner"

	return form.Init()
}

// --- Palette form values ---

// paletteFormValues holds values bound to the palette huh form fields.
type paletteFormValues struct {
	Selected string
}

// buildPaletteForm creates a huh form for selecting a color palette.
func (p *Plugin) buildPaletteForm() *huh.Form {
	p.paletteValues = &paletteFormValues{
		Selected: p.cfg.Palette,
	}

	names := config.PaletteNames()
	options := make([]huh.Option[string], len(names))
	for i, name := range names {
		label := name
		if name == p.cfg.Palette {
			label = name + " (active)"
		}
		options[i] = huh.NewOption(label, name)
	}

	// Capture pointer for the DescriptionFunc closure
	vals := p.paletteValues

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Color Palette").
				Options(options...).
				Value(&p.paletteValues.Selected),
			huh.NewNote().
				Title("Preview").
				DescriptionFunc(func() string {
					pal := config.GetPalette(vals.Selected, nil)
					return renderSwatches(pal)
				}, &vals.Selected),
		),
	).WithShowHelp(false).WithShowErrors(true).WithTheme(p.styles.huhTheme)

	return form
}

// savePaletteValues persists the current palette form value to config and
// rebuilds styles without rebuilding the form. Used for incremental auto-save.
func (p *Plugin) savePaletteValues() {
	if p.paletteValues == nil {
		return
	}

	selected := p.paletteValues.Selected
	previous := p.cfg.Palette
	if selected == previous {
		return // no change
	}

	p.cfg.Palette = selected

	if err := config.Save(p.cfg, true); err == nil {
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

// handlePaletteFormCompletion applies the selected palette, rebuilds styles, and publishes events.
func (p *Plugin) handlePaletteFormCompletion() tea.Cmd {
	if p.paletteValues == nil {
		return nil
	}

	vals := p.paletteValues
	p.paletteValues = nil

	selected := vals.Selected
	previous := p.cfg.Palette
	p.cfg.Palette = selected

	if err := config.Save(p.cfg, true); err == nil {
		// Rebuild all styles (including huh theme) so the palette change
		// is visible immediately.
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

	// Rebuild the form so it stays on screen with updated values and theme
	form := p.buildPaletteForm()
	// Apply updated theme to the new form
	form = form.WithTheme(p.styles.huhTheme)
	p.activeForm = form
	p.activeFormSlug = "palette"

	return form.Init()
}

// renderSwatches renders colored block swatches for a palette.
func renderSwatches(pal config.Palette) string {
	colors := []string{pal.Cyan, pal.Yellow, pal.Purple, pal.Green, pal.White}
	var parts []string
	for _, c := range colors {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
