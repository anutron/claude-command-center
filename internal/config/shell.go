package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const shellHookBegin = "# BEGIN CCC — managed by ccc, do not edit"
const shellHookEnd = "# END CCC"

const shellHookSnippet = `# BEGIN CCC — managed by ccc, do not edit
if [[ $- == *i* ]] && [[ -z "$CLAUDE_CODE" ]] && [[ -z "$CLAUDECODE" ]] && [[ -z "$CCC_SKIP" ]]; then
  # Also skip if an ancestor process is claude (catches agent-spawned terminals)
  _ccc_skip=0
  _ccc_pid=$$
  while [[ $_ccc_pid -gt 1 ]]; do
    _ccc_pid=$(ps -o ppid= -p "$_ccc_pid" 2>/dev/null | tr -d ' ')
    [[ -z "$_ccc_pid" ]] && break
    _ccc_comm=$(ps -o comm= -p "$_ccc_pid" 2>/dev/null)
    if [[ "$_ccc_comm" == *claude* ]]; then
      _ccc_skip=1
      break
    fi
  done
  if [[ $_ccc_skip -eq 0 ]]; then
    ccc
    _ccc_last_dir="$HOME/.config/ccc/data/last-dir"
    if [[ -f "$_ccc_last_dir" ]]; then
      cd "$(cat "$_ccc_last_dir")" 2>/dev/null
      rm -f "$_ccc_last_dir"
    fi
  fi
  unset _ccc_skip _ccc_pid _ccc_comm
fi
# END CCC`

// zshrcPath returns the path to ~/.zshrc.
func zshrcPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zshrc")
}

// IsShellHookInstalled checks if the CCC shell hook is present in .zshrc.
func IsShellHookInstalled() bool {
	data, err := os.ReadFile(zshrcPath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), shellHookBegin)
}

// InstallShellHook appends the CCC shell hook to .zshrc if not already present.
func InstallShellHook() error {
	if IsShellHookInstalled() {
		return nil
	}

	path := zshrcPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	// Add a newline before the hook for clean separation
	if _, err := f.WriteString("\n" + shellHookSnippet + "\n"); err != nil {
		return fmt.Errorf("writing shell hook: %w", err)
	}
	return nil
}

// UninstallShellHook removes the CCC shell hook from .zshrc.
func UninstallShellHook() error {
	path := zshrcPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	beginIdx := strings.Index(content, shellHookBegin)
	if beginIdx == -1 {
		return nil // not installed
	}

	endIdx := strings.Index(content, shellHookEnd)
	if endIdx == -1 {
		return nil // malformed, don't touch
	}
	endIdx += len(shellHookEnd)

	// Remove surrounding newlines
	if beginIdx > 0 && content[beginIdx-1] == '\n' {
		beginIdx--
	}
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}

	newContent := content[:beginIdx] + content[endIdx:]
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
