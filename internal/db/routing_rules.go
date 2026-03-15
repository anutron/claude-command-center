package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/anutron/claude-command-center/internal/config"
	"gopkg.in/yaml.v3"
)

// RoutingRule describes which tasks a path is or isn't relevant for.
type RoutingRule struct {
	UseFor []string `yaml:"use_for,omitempty"`
	NotFor []string `yaml:"not_for,omitempty"`
}

// routingRulesPath returns the path to the routing rules YAML file.
func routingRulesPath() string {
	return filepath.Join(config.ConfigDir(), "routing-rules.yaml")
}

// LoadRoutingRules parses the routing rules YAML file and returns a map
// keyed by path. Returns an empty map if the file is missing or malformed.
func LoadRoutingRules() (map[string]RoutingRule, error) {
	data, err := os.ReadFile(routingRulesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]RoutingRule{}, nil
		}
		return map[string]RoutingRule{}, nil
	}

	rules := make(map[string]RoutingRule)
	if err := yaml.Unmarshal(data, &rules); err != nil {
		log.Printf("WARNING: failed to parse %s: %v", routingRulesPath(), err)
		return map[string]RoutingRule{}, nil
	}
	return rules, nil
}

// SaveRoutingRules writes the rules map back to the YAML file.
// Creates parent directories if needed.
func SaveRoutingRules(rules map[string]RoutingRule) error {
	path := routingRulesPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshal routing rules: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// AddRoutingRule appends a use_for or not_for entry for a given path.
// Creates the file if it doesn't exist. ruleType must be "use_for" or "not_for".
func AddRoutingRule(path, ruleType, text string) error {
	if ruleType != "use_for" && ruleType != "not_for" {
		return fmt.Errorf("invalid rule type %q: must be \"use_for\" or \"not_for\"", ruleType)
	}

	rules, _ := LoadRoutingRules()

	rule := rules[path]
	switch ruleType {
	case "use_for":
		rule.UseFor = append(rule.UseFor, text)
	case "not_for":
		rule.NotFor = append(rule.NotFor, text)
	}
	rules[path] = rule

	return SaveRoutingRules(rules)
}
