package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunSetup runs an interactive setup wizard that creates a config.yaml.
func RunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Claude Command Center — Setup Wizard")
	fmt.Println()

	cfg := DefaultConfig()

	// Try loading existing config
	if existing, err := Load(); err == nil {
		cfg = existing
	}

	// 1. Name + palette
	fmt.Printf("Dashboard name [%s]: ", cfg.Name)
	if name := readLine(reader); name != "" {
		cfg.Name = name
	}

	fmt.Printf("Color palette (%s) [%s]: ", strings.Join(PaletteNames(), ", "), cfg.Palette)
	if palette := readLine(reader); palette != "" {
		cfg.Palette = palette
	}

	// 2. Calendar
	fmt.Println()
	if err := ValidateCalendar(); err != nil {
		fmt.Println("  [!!] Calendar: " + err.Error())
		fmt.Println("  To set up Google Calendar:")
		fmt.Println("    1. Create OAuth credentials at https://console.cloud.google.com/")
		fmt.Println("    2. Save credentials.json to ~/.config/google-calendar-mcp/credentials.json")
		fmt.Println("    3. Run the calendar MCP server once to complete OAuth flow")
	} else {
		fmt.Println("  [OK] Calendar credentials found")
		if !cfg.Calendar.Enabled {
			fmt.Print("  Enable calendar? [Y/n]: ")
			if ans := readLine(reader); ans == "" || strings.HasPrefix(strings.ToLower(ans), "y") {
				cfg.Calendar.Enabled = true
			}
		}
	}

	// 3. GitHub
	fmt.Println()
	if err := ValidateGitHub(); err != nil {
		fmt.Println("  [!!] GitHub: " + err.Error())
		fmt.Println("  Run 'gh auth login' to authenticate the GitHub CLI.")
	} else {
		fmt.Println("  [OK] GitHub CLI authenticated")
		if !cfg.GitHub.Enabled {
			fmt.Print("  Enable GitHub? [Y/n]: ")
			if ans := readLine(reader); ans == "" || strings.HasPrefix(strings.ToLower(ans), "y") {
				cfg.GitHub.Enabled = true
			}
		}
	}

	// 4. Granola
	fmt.Println()
	if err := ValidateGranola(); err != nil {
		fmt.Println("  [!!] Granola: " + err.Error())
	} else {
		fmt.Println("  [OK] Granola configured")
		if !cfg.Granola.Enabled {
			fmt.Print("  Enable Granola? [Y/n]: ")
			if ans := readLine(reader); ans == "" || strings.HasPrefix(strings.ToLower(ans), "y") {
				cfg.Granola.Enabled = true
			}
		}
	}

	// Save config
	if err := Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConfig saved to %s\n", ConfigPath())

	// 5. MCP config generation
	fmt.Println()
	fmt.Print("Generate MCP server config for Claude Code? [Y/n]: ")
	if ans := readLine(reader); ans == "" || strings.HasPrefix(strings.ToLower(ans), "y") {
		if err := generateMCPConfig(); err != nil {
			fmt.Printf("  [!!] MCP config: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Run 'ccc' to launch the dashboard.")
	fmt.Println("Run 'ccc doctor' to verify your setup.")
	return nil
}

// generateMCPConfig writes MCP server entries for gmail and things to ~/.claude/mcp.json.
func generateMCPConfig() error {
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

	// Things MCP
	thingsEntry := filepath.Join(serversDir, "things", "dist", "index.js")
	if _, err := os.Stat(thingsEntry); err == nil {
		servers["things"] = map[string]interface{}{
			"command": "node",
			"args":    []string{thingsEntry},
		}
		added = append(added, "things")
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

	fmt.Printf("  [OK] Added %s to %s\n", strings.Join(added, ", "), mcpPath)
	return nil
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

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
