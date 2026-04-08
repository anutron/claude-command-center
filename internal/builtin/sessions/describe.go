package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anutron/claude-command-center/internal/db"
	"github.com/anutron/claude-command-center/internal/llm"
	tea "github.com/charmbracelet/bubbletea"
)

// pathDescribeFinishedMsg is returned by pathDescribeCmd when the LLM
// (or heuristic fallback) finishes generating a project description.
type pathDescribeFinishedMsg struct {
	path        string
	description string
	err         error
}

// LLMDescribePath reads README.md and CLAUDE.md from dir, then calls the LLM
// to produce a 1-2 sentence project summary. Falls back to AutoDescribePath
// heuristics if the LLM returns an error.
func LLMDescribePath(l llm.LLM, dir string) (string, error) {
	readme := readFileHead(filepath.Join(dir, "README.md"), 200)
	claudeMD := readFileHead(filepath.Join(dir, "CLAUDE.md"), 100)

	if readme == "" && claudeMD == "" {
		// No project files to summarize — use heuristic
		return db.AutoDescribePath(dir), nil
	}

	prompt := buildDescribePrompt(readme, claudeMD)
	desc, err := l.Complete(llm.WithOperation(context.Background(), "describe"), prompt)
	if err != nil {
		// LLM failed — fall back to heuristic
		fallback := db.AutoDescribePath(dir)
		return fallback, fmt.Errorf("LLM describe failed (using heuristic): %w", err)
	}

	desc = strings.TrimSpace(desc)
	if desc == "" {
		return db.AutoDescribePath(dir), nil
	}
	return desc, nil
}

// pathDescribeCmd returns an async tea.Cmd that calls LLMDescribePath and
// wraps the result in a pathDescribeFinishedMsg.
func pathDescribeCmd(l llm.LLM, path string) tea.Cmd {
	return func() tea.Msg {
		desc, err := LLMDescribePath(l, path)
		return pathDescribeFinishedMsg{
			path:        path,
			description: desc,
			err:         err,
		}
	}
}

func buildDescribePrompt(readme, claudeMD string) string {
	var sb strings.Builder
	sb.WriteString(`Summarize this project in 1-2 sentences for someone deciding which project
directory to route a task to. Include: what the project does, primary tech
stack, and domain. Be specific — "Go TUI dashboard for personal productivity"
is better than "a software project."

`)
	if readme != "" {
		sb.WriteString("README.md:\n")
		sb.WriteString(readme)
		sb.WriteString("\n\n")
	}
	if claudeMD != "" {
		sb.WriteString("CLAUDE.md (project instructions):\n")
		sb.WriteString(claudeMD)
		sb.WriteString("\n")
	}
	return sb.String()
}

// readFileHead reads up to maxLines lines from a file. Returns empty string
// if the file doesn't exist or can't be read.
func readFileHead(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}
