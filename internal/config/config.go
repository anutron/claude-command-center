package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name            string        `yaml:"name"`
	HomeDir         string        `yaml:"home_dir,omitempty"`
	Subtitle        string        `yaml:"subtitle,omitempty"`
	ShowBanner      *bool         `yaml:"show_banner,omitempty"`
	BannerTopPadding *int        `yaml:"banner_top_padding,omitempty"`
	Palette         string        `yaml:"palette"`
	Colors          *CustomColors `yaml:"colors,omitempty"`
	RefreshInterval string        `yaml:"refresh_interval,omitempty"`

	Calendar        CalendarConfig         `yaml:"calendar"`
	GitHub          GitHubConfig           `yaml:"github"`
	Todos           TodosConfig            `yaml:"todos"`
	Threads         ThreadsConfig          `yaml:"threads"`
	Granola         GranolaConfig          `yaml:"granola"`
	Slack           SlackConfig            `yaml:"slack"`
	Gmail           GmailConfig            `yaml:"gmail"`
	ExternalPlugins []ExternalPluginConfig `yaml:"external_plugins"`

	// DisabledPlugins lists slugs of built-in plugins the user has turned off.
	// e.g. ["sessions", "commandcenter"]
	DisabledPlugins []string `yaml:"disabled_plugins,omitempty"`

	// loadedFromFile is true when the config was successfully loaded from disk.
	// When false (i.e. defaults), Save will refuse to overwrite an existing file.
	loadedFromFile bool `yaml:"-"`
}

// PluginEnabled returns whether a built-in plugin is enabled (not in DisabledPlugins).
func (c *Config) PluginEnabled(slug string) bool {
	for _, s := range c.DisabledPlugins {
		if s == slug {
			return false
		}
	}
	return true
}

// SetPluginEnabled adds or removes a slug from DisabledPlugins.
func (c *Config) SetPluginEnabled(slug string, enabled bool) {
	if enabled {
		// Remove from disabled list
		out := c.DisabledPlugins[:0]
		for _, s := range c.DisabledPlugins {
			if s != slug {
				out = append(out, s)
			}
		}
		c.DisabledPlugins = out
	} else {
		// Add to disabled list if not already there
		if c.PluginEnabled(slug) {
			c.DisabledPlugins = append(c.DisabledPlugins, slug)
		}
	}
}

const DefaultRefreshInterval = 5 * time.Minute

// ParseRefreshInterval returns the configured refresh interval, or the default.
func (c *Config) ParseRefreshInterval() time.Duration {
	if c.RefreshInterval == "" {
		return DefaultRefreshInterval
	}
	d, err := time.ParseDuration(c.RefreshInterval)
	if err != nil || d < 1*time.Minute {
		return DefaultRefreshInterval
	}
	return d
}

type CustomColors struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Accent    string `yaml:"accent"`
}

type CalendarConfig struct {
	Enabled   bool            `yaml:"enabled"`
	Calendars []CalendarEntry `yaml:"calendars"`
}

type CalendarEntry struct {
	ID      string `yaml:"id"`
	Label   string `yaml:"label"`
	Color   string `yaml:"color,omitempty"`
	Enabled *bool  `yaml:"enabled,omitempty"`
}

// IsEnabled returns whether this calendar entry is enabled.
// Defaults to true if the Enabled field is nil (not set).
func (e CalendarEntry) IsEnabled() bool {
	if e.Enabled == nil {
		return true
	}
	return *e.Enabled
}

// SetEnabled sets the enabled state of a calendar entry.
func (e *CalendarEntry) SetEnabled(v bool) {
	e.Enabled = &v
}

type GitHubConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Repos    []string `yaml:"repos"`
	Username string   `yaml:"username"`
}

type TodosConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ThreadsConfig struct {
	Enabled bool `yaml:"enabled"`
}

type GranolaConfig struct {
	Enabled bool `yaml:"enabled"`
}

type SlackConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"bot_token,omitempty"`
}

type GmailConfig struct {
	Enabled   bool   `yaml:"enabled"`
	TodoLabel string `yaml:"todo_label,omitempty"` // Gmail label name to sync as todos (empty = disabled)
	Advanced  bool   `yaml:"advanced,omitempty"`   // opt-in for modify+compose scopes (label mgmt, drafts)
}

type ExternalPluginConfig struct {
	Name        string `yaml:"name"`
	Command     string `yaml:"command"`
	Description string `yaml:"description,omitempty"`
	Enabled     bool   `yaml:"enabled"`
}

// BannerVisible returns whether the banner should be shown.
// Defaults to true if ShowBanner is nil (backwards compat).
func (c *Config) BannerVisible() bool {
	if c.ShowBanner == nil {
		return true
	}
	return *c.ShowBanner
}

// SetShowBanner sets the ShowBanner field.
func (c *Config) SetShowBanner(v bool) {
	c.ShowBanner = &v
}

// GetBannerTopPadding returns the number of blank lines above the banner.
// Defaults to 10 if not set.
func (c *Config) GetBannerTopPadding() int {
	if c.BannerTopPadding == nil {
		return 10
	}
	return *c.BannerTopPadding
}

// SetBannerTopPadding sets the BannerTopPadding field.
func (c *Config) SetBannerTopPadding(v int) {
	if v < 0 {
		v = 0
	}
	c.BannerTopPadding = &v
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Name:    "Claude Command",
		Palette: "aurora",
		Todos:   TodosConfig{Enabled: true},
		Threads: ThreadsConfig{Enabled: true},
	}
}

// ConfigDir returns the configuration directory path.
// Uses $CCC_CONFIG_DIR if set, otherwise ~/.config/ccc.
func ConfigDir() string {
	if dir := os.Getenv("CCC_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ccc")
}

// ConfigPath returns the path to config.yaml.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// DataDir returns the data directory path.
// Uses $CCC_STATE_DIR if set, otherwise ConfigDir()/data.
func DataDir() string {
	if dir := os.Getenv("CCC_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(ConfigDir(), "data")
}

// DBPath returns the path to the SQLite database.
func DBPath() string {
	return filepath.Join(DataDir(), "ccc.db")
}

// CredentialsDir returns the path to the credentials directory.
func CredentialsDir() string {
	return filepath.Join(ConfigDir(), "credentials")
}

// Load reads the config from ConfigPath(). If the file doesn't exist,
// it returns DefaultConfig() without error.
func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ConfigPath(), err)
	}
	cfg.loadedFromFile = true
	return cfg, nil
}

// MarkLoadedFromFile marks a config as having been loaded from or saved to
// disk, so that subsequent Save calls are allowed to write it. This should
// only be called after a successful first Save (e.g. during onboarding).
func (c *Config) MarkLoadedFromFile() {
	c.loadedFromFile = true
}

// Save writes the config to ConfigPath(), creating directories as needed.
// If the config was not loaded from a file (i.e. it is a default config due to
// a load error), Save refuses to overwrite an existing config file to prevent
// data loss. It also creates a .bak backup before writing as defense-in-depth.
func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := ConfigPath()

	// Safety check: refuse to overwrite an existing config with defaults.
	// This prevents the scenario where Load() fails, the app falls back to
	// DefaultConfig(), and then a settings change writes defaults to disk.
	if !cfg.loadedFromFile {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("refusing to overwrite %s: config was not loaded from file (possible data loss)", path)
		}
	}

	// Create a backup of the existing file before writing.
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 {
		_ = os.WriteFile(path+".bak", existing, 0o644)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Write to a temp file and rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Rename failed — fall back to direct write.
		return os.WriteFile(path, data, 0o644)
	}

	cfg.loadedFromFile = true
	return nil
}
