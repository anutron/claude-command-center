package external

import (
	"os/exec"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
)

// resolveCommand tries the given command as-is, then with a "ccc-" prefix
// on the first word (binary name). This lets config entries use short names
// like "pomodoro" while the installed binary is "ccc-pomodoro".
func resolveCommand(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return command
	}
	// If the command is found on PATH as-is, use it.
	if _, err := exec.LookPath(parts[0]); err == nil {
		return command
	}
	// Try with "ccc-" prefix.
	prefixed := "ccc-" + parts[0]
	if _, err := exec.LookPath(prefixed); err == nil {
		parts[0] = prefixed
		return strings.Join(parts, " ")
	}
	// Return original; startProcess will report the not-found error.
	return command
}

// LoadExternalPlugins creates and initializes ExternalPlugin instances
// from the config's ExternalPlugins entries.
func LoadExternalPlugins(cfg *config.Config, ctx plugin.Context) ([]*ExternalPlugin, error) {
	var plugins []*ExternalPlugin

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

		plugins = append(plugins, ep)
	}

	return plugins, nil
}
