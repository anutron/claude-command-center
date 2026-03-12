package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/worktree"
)

func runWorktrees(args []string) error {
	if len(args) > 0 && args[0] == "prune" {
		return runWorktreesPrune(args[1:])
	}
	return runWorktreesList()
}

func runWorktreesList() error {
	paths, err := getKnownPaths()
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		fmt.Println("No known project paths. Launch some sessions first.")
		return nil
	}

	found := false
	for _, p := range paths {
		wts, err := worktree.ListWorktrees(p)
		if err != nil {
			continue
		}
		if len(wts) == 0 {
			continue
		}
		found = true
		fmt.Println(filepath.Base(p))
		for _, wt := range wts {
			age := formatAge(wt.CreatedAt)
			relPath := strings.TrimPrefix(wt.Path, wt.RepoRoot+"/")
			fmt.Printf("  %-30s %s  (%s)\n", wt.Branch, relPath, age)
		}
	}

	if !found {
		fmt.Println("No CCC worktrees found.")
	}
	return nil
}

func runWorktreesPrune(args []string) error {
	var paths []string

	if len(args) > 0 {
		// Prune specific repo path.
		paths = []string{args[0]}
	} else {
		var err error
		paths, err = getKnownPaths()
		if err != nil {
			return err
		}
	}

	// Collect all worktrees to prune.
	type pruneTarget struct {
		repoRoot string
		wt       worktree.WorktreeInfo
	}
	var targets []pruneTarget
	for _, p := range paths {
		wts, err := worktree.ListWorktrees(p)
		if err != nil {
			continue
		}
		for _, wt := range wts {
			targets = append(targets, pruneTarget{repoRoot: p, wt: wt})
		}
	}

	if len(targets) == 0 {
		fmt.Println("No CCC worktrees to prune.")
		return nil
	}

	fmt.Printf("Will remove %d worktree(s):\n", len(targets))
	for _, t := range targets {
		fmt.Printf("  %s  (%s)\n", t.wt.Branch, t.wt.Path)
	}
	fmt.Print("\nProceed? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		fmt.Println("Aborted.")
		return nil
	}

	for _, t := range targets {
		if err := worktree.RemoveWorktree(t.repoRoot, t.wt.Path); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to remove %s: %v\n", t.wt.Path, err)
			continue
		}
		fmt.Printf("  removed %s\n", t.wt.Branch)
	}

	return nil
}

// getKnownPaths loads learned project paths from the database.
func getKnownPaths() ([]string, error) {
	dbPath := config.DBPath()
	database, err := db.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	defer database.Close()

	return db.DBLoadPaths(database)
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
