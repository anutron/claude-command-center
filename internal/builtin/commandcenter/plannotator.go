package commandcenter

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// plannotatorFinishedMsg is sent when the external editor/plannotator process exits.
type plannotatorFinishedMsg struct {
	todoID   string
	tempFile string
	err      error
}

// editorProcess implements tea.ExecCommand to launch an editor on a temp file.
type editorProcess struct {
	tempFile string
	stdin    io.Reader
	stderr   io.Writer
}

func (e *editorProcess) SetStdin(r io.Reader)  { e.stdin = r }
func (e *editorProcess) SetStdout(_ io.Writer) {}
func (e *editorProcess) SetStderr(w io.Writer) { e.stderr = w }

func (e *editorProcess) Run() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd := exec.Command(editor, e.tempFile)
	cmd.Stdin = e.stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = e.stderr
	return cmd.Run()
}

// writeTempPrompt writes the prompt content to a temp file and returns the path.
func writeTempPrompt(todoID, content string) (string, error) {
	path := fmt.Sprintf("/tmp/ccc-prompt-%s.md", todoID)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// readTempPrompt reads the prompt back from the temp file.
// Returns empty string if file doesn't exist or is empty.
func readTempPrompt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// launchPlannotator writes the prompt to a temp file and returns a tea.Cmd
// that suspends the TUI to launch an editor on the file.
func launchPlannotator(todoID, prompt string) tea.Cmd {
	tempFile, err := writeTempPrompt(todoID, prompt)
	if err != nil {
		return func() tea.Msg {
			return plannotatorFinishedMsg{todoID: todoID, err: err}
		}
	}

	proc := &editorProcess{tempFile: tempFile}
	return tea.Exec(proc, func(err error) tea.Msg {
		return plannotatorFinishedMsg{todoID: todoID, tempFile: tempFile, err: err}
	})
}
