package auth

import (
	"os"
	"strings"
)

// envFilePath returns the path to the CCC .env file.
func envFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + "/.config/ccc/.env", nil
}

// LoadEnvFile reads KEY=VALUE pairs from ~/.config/ccc/.env and sets them as env vars.
// Existing environment variables are not overwritten.
func LoadEnvFile() {
	path, err := envFilePath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// ReadEnv reads a single value from the CCC .env file without modifying the environment.
// Returns empty string if the key is not found or the file doesn't exist.
func ReadEnv(key string) string {
	path, err := envFilePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, "\"'")
		if k == key {
			return v
		}
	}
	return ""
}

// WriteEnvValue writes or updates a KEY=VALUE pair in the CCC .env file.
// If the key already exists, its value is updated in place.
// If the key does not exist, it is appended.
// The file is created (along with parent directories) if it doesn't exist.
func WriteEnvValue(key, value string) error {
	path, err := envFilePath()
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := path[:strings.LastIndex(path, "/")]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, _ := os.ReadFile(path) // ok if not found
	lines := strings.Split(string(data), "\n")

	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == key {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}

	if !found {
		// Append, ensuring there's a newline before the new entry
		content := strings.TrimRight(string(data), "\n")
		if content != "" {
			content += "\n"
		}
		content += key + "=" + value + "\n"
		return os.WriteFile(path, []byte(content), 0o600)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
