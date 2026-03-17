package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/anutron/claude-command-center/internal/builtin/sessions"
	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
)

// pathsJSONOutput is the top-level JSON structure for ccc paths --json.
type pathsJSONOutput struct {
	Paths        []pathsJSONEntry `json:"paths"`
	GlobalSkills []db.SkillInfo   `json:"global_skills"`
}

// pathsJSONEntry represents a single learned path with metadata.
type pathsJSONEntry struct {
	Path         string         `json:"path"`
	Description  string         `json:"description"`
	AddedAt      string         `json:"added_at"`
	SortOrder    int            `json:"sort_order"`
	Skills       []db.SkillInfo `json:"skills"`
	RoutingRules *db.RoutingRule `json:"routing_rules,omitempty"`
}

func runPaths(args []string) error {
	fs := flag.NewFlagSet("paths", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "Output as JSON")
	refreshSkills := fs.Bool("refresh-skills", false, "Force skill cache refresh")
	autoDescribe := fs.Bool("auto-describe", false, "Generate descriptions for paths without one")
	addRule := fs.String("add-rule", "", "Path to add a routing rule for")
	useFor := fs.String("use-for", "", "Add a use_for routing rule (requires --add-rule)")
	notFor := fs.String("not-for", "", "Add a not_for routing rule (requires --add-rule)")
	promptHint := fs.String("prompt-hint", "", "Set prompt generation hint (requires --add-rule)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Handle --add-rule mode
	if *addRule != "" {
		return runAddRule(*addRule, *useFor, *notFor, *promptHint)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	entries, err := db.DBLoadPathsFull(database)
	if err != nil {
		return fmt.Errorf("load paths: %w", err)
	}

	// Handle --auto-describe: generate descriptions for paths that lack one
	if *autoDescribe {
		return runAutoDescribe(database, entries)
	}

	if *jsonFlag {
		return runPathsJSON(entries, *refreshSkills)
	}

	// Default: plain text listing
	if len(entries) == 0 {
		fmt.Println("No learned paths. Launch some sessions first.")
		return nil
	}
	for _, e := range entries {
		if e.Description != "" {
			fmt.Printf("%s — %s\n", e.Path, e.Description)
		} else {
			fmt.Println(e.Path)
		}
	}
	return nil
}

func runPathsJSON(entries []db.PathEntry, refreshSkills bool) error {
	routingRules, _ := db.LoadRoutingRules()

	var jsonEntries []pathsJSONEntry
	for _, e := range entries {
		je := pathsJSONEntry{
			Path:        e.Path,
			Description: e.Description,
			AddedAt:     e.AddedAt.Format("2006-01-02T15:04:05Z"),
			SortOrder:   e.SortOrder,
		}

		// Load project skills
		skills, _ := db.GetProjectSkills(e.Path, refreshSkills)
		if skills == nil {
			skills = []db.SkillInfo{}
		}
		je.Skills = skills

		// Look up routing rules
		if rule, ok := routingRules[e.Path]; ok {
			je.RoutingRules = &rule
		}

		jsonEntries = append(jsonEntries, je)
	}
	if jsonEntries == nil {
		jsonEntries = []pathsJSONEntry{}
	}

	// Load global skills
	globalSkills, _ := db.GetGlobalSkills(refreshSkills)
	if globalSkills == nil {
		globalSkills = []db.SkillInfo{}
	}

	output := pathsJSONOutput{
		Paths:        jsonEntries,
		GlobalSkills: globalSkills,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func runAddRule(path, useFor, notFor, promptHint string) error {
	if useFor == "" && notFor == "" && promptHint == "" {
		return fmt.Errorf("--add-rule requires --use-for, --not-for, or --prompt-hint")
	}
	if useFor != "" {
		if err := db.AddRoutingRule(path, "use_for", useFor); err != nil {
			return fmt.Errorf("add use_for rule: %w", err)
		}
		fmt.Printf("Added use_for rule for %s: %s\n", path, useFor)
	}
	if notFor != "" {
		if err := db.AddRoutingRule(path, "not_for", notFor); err != nil {
			return fmt.Errorf("add not_for rule: %w", err)
		}
		fmt.Printf("Added not_for rule for %s: %s\n", path, notFor)
	}
	if promptHint != "" {
		if err := db.SetPromptHint(path, promptHint); err != nil {
			return fmt.Errorf("set prompt hint: %w", err)
		}
		fmt.Printf("Set prompt_hint for %s: %s\n", path, promptHint)
	}
	return nil
}

func runAutoDescribe(database *sql.DB, entries []db.PathEntry) error {
	// Determine LLM availability
	var l llm.LLM
	if llm.Available() {
		l = llm.ClaudeCLI{}
		fmt.Println("Using LLM for descriptions...")
	} else {
		fmt.Println("claude CLI not found — using heuristic descriptions only")
	}

	updated := 0
	for _, e := range entries {
		if e.Description != "" {
			continue // already has a description
		}

		var desc string
		if l != nil {
			d, err := sessions.LLMDescribePath(l, e.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: %s: %v\n", e.Path, err)
			}
			desc = d
		} else {
			desc = db.AutoDescribePath(e.Path)
		}

		if desc == "" {
			continue
		}

		if err := db.DBUpdatePathDescription(database, e.Path, desc); err != nil {
			fmt.Fprintf(os.Stderr, "  error updating %s: %v\n", e.Path, err)
			continue
		}
		fmt.Printf("  %s → %s\n", e.Path, desc)
		updated++
	}

	fmt.Printf("Updated %d path(s).\n", updated)
	return nil
}
