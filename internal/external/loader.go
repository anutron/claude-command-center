package external

import (
	"fmt"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// reservedSlugs are built-in plugin slugs that external plugins must not use.
var reservedSlugs = map[string]bool{
	"sessions":      true,
	"commandcenter": true,
	"settings":      true,
}

// resolveCommand returns the command as-is. Plugin configs must specify
// the full binary name (e.g. "ccc-pomodoro", not "pomodoro").
// Previously this function tried a "ccc-" prefix fallback, which was
// removed because PATH manipulation could shadow arbitrary binaries.
func resolveCommand(command string) string {
	return command
}

// LoadExternalPlugins creates and initializes ExternalPlugin instances
// from the config's ExternalPlugins entries. Rejects plugins whose slugs
// collide with reserved built-in slugs or with already-loaded plugins.
func LoadExternalPlugins(cfg *config.Config, ctx plugin.Context) ([]*ExternalPlugin, error) {
	var plugins []*ExternalPlugin
	loadedSlugs := make(map[string]bool)

	for _, entry := range cfg.ExternalPlugins {
		if !entry.Enabled || entry.Command == "" {
			continue
		}

		ep := &ExternalPlugin{
			command: resolveCommand(entry.Command),
			tabName: entry.Name,
		}

		if err := ep.Init(ctx); err != nil {
			if ctx.Logger != nil {
				ctx.Logger.Warn("external", "failed to load plugin "+entry.Command+": "+err.Error())
			}
			// Keep the plugin in the list so it appears with an error view
			// and the user can press 'r' to retry.
			if ep.slug == "" {
				ep.slug = entry.Name
			}
		}

		// Validate slug against reserved built-in slugs.
		if reservedSlugs[ep.slug] {
			if ctx.Logger != nil {
				ctx.Logger.Error("external", fmt.Sprintf(
					"plugin %q rejected: slug %q is reserved for a built-in plugin",
					entry.Command, ep.slug))
			}
			ep.Shutdown()
			continue
		}

		// Validate slug uniqueness among already-loaded external plugins.
		if loadedSlugs[ep.slug] {
			if ctx.Logger != nil {
				ctx.Logger.Error("external", fmt.Sprintf(
					"plugin %q rejected: slug %q is already in use by another plugin",
					entry.Command, ep.slug))
			}
			ep.Shutdown()
			continue
		}

		loadedSlugs[ep.slug] = true
		plugins = append(plugins, ep)
	}

	return plugins, nil
}
