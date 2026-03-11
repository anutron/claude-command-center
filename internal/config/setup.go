package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// IsMCPBuilt returns a map of server name to whether its dist/index.js exists.
func IsMCPBuilt() map[string]bool {
	result := map[string]bool{}
	serversDir := findServersDir()
	if serversDir == "" {
		return result
	}
	for _, name := range []string{"gmail"} {
		entryPoint := filepath.Join(serversDir, name, "dist", "index.js")
		_, err := os.Stat(entryPoint)
		result[name] = err == nil
	}
	return result
}

// GenerateMCPConfig writes MCP server entries for gmail to ~/.claude/mcp.json.
func GenerateMCPConfig() error {
	// Find the servers directory (next to the ccc binary or in the repo)
	serversDir := findServersDir()
	if serversDir == "" {
		return fmt.Errorf("servers/ directory not found — run 'make servers' first")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	mcpPath := filepath.Join(home, ".claude", "mcp.json")

	// Load existing mcp.json if present
	var mcpConfig map[string]interface{}
	if data, err := os.ReadFile(mcpPath); err == nil {
		if err := json.Unmarshal(data, &mcpConfig); err != nil {
			mcpConfig = make(map[string]interface{})
		}
	} else {
		mcpConfig = make(map[string]interface{})
	}

	// Ensure mcpServers key exists
	servers, ok := mcpConfig["mcpServers"].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
	}

	added := []string{}

	// Gmail MCP
	gmailEntry := filepath.Join(serversDir, "gmail", "dist", "index.js")
	if _, err := os.Stat(gmailEntry); err == nil {
		servers["gmail"] = map[string]interface{}{
			"command": "node",
			"args":    []string{gmailEntry},
		}
		added = append(added, "gmail")
	}

	if len(added) == 0 {
		return fmt.Errorf("no built MCP servers found in %s — run 'make servers'", serversDir)
	}

	mcpConfig["mcpServers"] = servers

	// Write mcp.json
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(mcpPath, data, 0o644); err != nil {
		return err
	}

	return nil
}

// BuildAndConfigureMCP builds MCP servers if needed and writes their config to ~/.claude/mcp.json.
// Returns the list of servers that were configured.
func BuildAndConfigureMCP() ([]string, error) {
	// Check node is available
	if _, err := exec.LookPath("node"); err != nil {
		return nil, fmt.Errorf("node not found")
	}

	serversDir := findServersDir()
	if serversDir == "" {
		return nil, fmt.Errorf("servers/ directory not found")
	}

	var added []string
	serverNames := []string{"gmail"}

	for _, name := range serverNames {
		serverDir := filepath.Join(serversDir, name)
		entryPoint := filepath.Join(serverDir, "dist", "index.js")

		// Skip build if dist/index.js already exists
		if _, err := os.Stat(entryPoint); err == nil {
			added = append(added, name)
			continue
		}

		// Build the server: npm install && npm run build
		install := exec.Command("npm", "install")
		install.Dir = serverDir
		if out, err := install.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("npm install for %s failed: %w\n%s", name, err, string(out))
		}

		build := exec.Command("npm", "run", "build")
		build.Dir = serverDir
		if out, err := build.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("npm run build for %s failed: %w\n%s", name, err, string(out))
		}

		added = append(added, name)
	}

	// Write MCP config
	if err := GenerateMCPConfig(); err != nil {
		return nil, fmt.Errorf("generating MCP config: %w", err)
	}

	return added, nil
}

// findServersDir looks for the servers/ directory relative to the binary or cwd.
func findServersDir() string {
	// Next to the current executable
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "servers")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "servers")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}
