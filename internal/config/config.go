package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name            string        `yaml:"name"`
	Palette         string        `yaml:"palette"`
	Colors          *CustomColors `yaml:"colors,omitempty"`
	RefreshInterval string        `yaml:"refresh_interval,omitempty"`

	Calendar        CalendarConfig         `yaml:"calendar"`
	GitHub          GitHubConfig           `yaml:"github"`
	Todos           TodosConfig            `yaml:"todos"`
	Granola         GranolaConfig          `yaml:"granola"`
	ExternalPlugins []ExternalPluginConfig `yaml:"external_plugins"`
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

type GranolaConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ExternalPluginConfig struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Enabled bool   `yaml:"enabled"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Name:    "Command Center",
		Palette: "aurora",
		Todos:   TodosConfig{Enabled: true},
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
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to ConfigPath(), creating directories as needed.
func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0o644)
}
