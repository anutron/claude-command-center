package config

import (
	"bufio"
	"fmt"
	"os"
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
	fmt.Println("Run 'ccc' to launch the dashboard.")
	fmt.Println("Run 'ccc doctor' to verify your setup.")
	return nil
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
