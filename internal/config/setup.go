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

	// Name
	fmt.Printf("Dashboard name [%s]: ", cfg.Name)
	if name := readLine(reader); name != "" {
		cfg.Name = name
	}

	// Palette
	fmt.Printf("Color palette (%s) [%s]: ", strings.Join(PaletteNames(), ", "), cfg.Palette)
	if palette := readLine(reader); palette != "" {
		cfg.Palette = palette
	}

	// Save
	if err := Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConfig saved to %s\n", ConfigPath())
	fmt.Println("Run 'ccc' to launch the dashboard.")
	return nil
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
