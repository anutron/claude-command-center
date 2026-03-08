package external

import (
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// LoadExternalPlugins creates and initializes ExternalPlugin instances
// from the config's ExternalPlugins entries.
func LoadExternalPlugins(cfg *config.Config, ctx plugin.Context) ([]*ExternalPlugin, error) {
	var plugins []*ExternalPlugin

	for _, entry := range cfg.ExternalPlugins {
		if !entry.Enabled || entry.Command == "" {
			continue
		}

		ep := &ExternalPlugin{
			command: entry.Command,
			tabName: entry.Name,
		}

		if err := ep.Init(ctx); err != nil {
			if ctx.Logger != nil {
				ctx.Logger.Warn("external", "failed to load plugin "+entry.Command+": "+err.Error())
			}
			continue
		}

		plugins = append(plugins, ep)
	}

	return plugins, nil
}
