package plugin

import (
	"reflect"
	"strings"

	"github.com/anutron/claude-command-center/internal/config"
)

// ScopeConfig returns a map containing only the top-level config fields
// matching the requested scopes. Uses reflection to extract fields from the
// Config struct by matching yaml tags to scope names. If scopes is empty,
// returns an empty map (secure by default).
func ScopeConfig(cfg *config.Config, scopes []string) map[string]interface{} {
	if len(scopes) == 0 || cfg == nil {
		return map[string]interface{}{}
	}

	allowed := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		allowed[strings.ToLower(s)] = true
	}

	result := make(map[string]interface{})
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		// Parse yaml tag to get the field name (before any comma options).
		name := strings.Split(tag, ",")[0]
		if allowed[name] {
			result[name] = v.Field(i).Interface()
		}
	}
	return result
}
