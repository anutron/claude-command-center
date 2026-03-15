package db

import (
	"os"
	"path/filepath"
	"strings"
)

// AutoDescribePath generates a heuristic description of a project directory
// by examining common project files (package.json, go.mod, Cargo.toml, etc.).
// Returns an empty string if no recognizable project files are found.
func AutoDescribePath(dir string) string {
	var parts []string

	// Check for language/framework indicators
	if fileContains(filepath.Join(dir, "go.mod"), "") {
		mod := readFirstLine(filepath.Join(dir, "go.mod"))
		parts = append(parts, "Go project")
		if strings.Contains(mod, "module ") {
			modName := strings.TrimPrefix(mod, "module ")
			parts = append(parts, "("+strings.TrimSpace(modName)+")")
		}
	} else if fileExists(filepath.Join(dir, "package.json")) {
		parts = append(parts, "Node.js/JavaScript project")
	} else if fileExists(filepath.Join(dir, "Cargo.toml")) {
		parts = append(parts, "Rust project")
	} else if fileExists(filepath.Join(dir, "pyproject.toml")) || fileExists(filepath.Join(dir, "setup.py")) {
		parts = append(parts, "Python project")
	} else if fileExists(filepath.Join(dir, "Gemfile")) {
		parts = append(parts, "Ruby project")
	} else if fileExists(filepath.Join(dir, "pom.xml")) || fileExists(filepath.Join(dir, "build.gradle")) {
		parts = append(parts, "Java project")
	} else if fileExists(filepath.Join(dir, "Package.swift")) {
		parts = append(parts, "Swift project")
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " ")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileContains(path, _ string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFirstLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) > 0 {
		return lines[0]
	}
	return ""
}
