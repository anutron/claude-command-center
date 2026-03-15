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

// plannotatorReviewMsg is sent when the Plannotator review process exits.
type plannotatorReviewMsg struct {
	todoID   string
	tempFile string
	err      error
	round    int
	feedback string // user's annotations (deny case)
	approved bool   // true if user clicked approve
}

// plannotatorReviewProcess implements tea.ExecCommand to run plannotator in
// plan review mode. Pipes the prompt as a hook event on stdin, gets approve/deny
// with feedback on stdout. Opens the browser with approve/deny buttons.
type plannotatorReviewProcess struct {
	prompt   string
	stdin    io.Reader
	stderr   io.Writer
	approved bool
	feedback string
}

func (p *plannotatorReviewProcess) SetStdin(r io.Reader)  { p.stdin = r }
func (p *plannotatorReviewProcess) SetStdout(_ io.Writer) {}
func (p *plannotatorReviewProcess) SetStderr(w io.Writer) { p.stderr = w }

func (p *plannotatorReviewProcess) Run() error {
	// Plannotator plan review mode reads a hook event from stdin.
	hookEvent := fmt.Sprintf(`{"tool_input":{"plan":%q},"permission_mode":"default"}`, p.prompt)

	cmd := exec.Command("plannotator")
	cmd.Stdin = strings.NewReader(hookEvent)
	cmd.Stderr = p.stderr
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse the JSON response to extract approved/feedback.
	// Response format: {"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"|"deny","message":"..."}}}
	outStr := strings.TrimSpace(string(out))
	p.approved = strings.Contains(outStr, `"behavior":"allow"`)
	if !p.approved {
		// Extract the feedback message from the deny decision.
		// The message field contains the full feedback after the preamble.
		if idx := strings.Index(outStr, `"message":"`); idx >= 0 {
			rest := outStr[idx+len(`"message":"`):]
			if end := strings.Index(rest, `"}`); end >= 0 {
				p.feedback = rest[:end]
			} else {
				p.feedback = rest
			}
			// Unescape JSON string
			p.feedback = strings.ReplaceAll(p.feedback, `\n`, "\n")
			p.feedback = strings.ReplaceAll(p.feedback, `\"`, `"`)
		}
	}
	return nil
}

// launchPlannotatorReview opens Plannotator in plan review mode with the prompt.
// The user gets approve/deny buttons in the browser. Returns plannotatorReviewMsg.
func launchPlannotatorReview(todoID string, prompt string, round int) tea.Cmd {
	proc := &plannotatorReviewProcess{prompt: prompt}
	return tea.Exec(proc, func(err error) tea.Msg {
		return plannotatorReviewMsg{
			todoID:   todoID,
			err:      err,
			round:    round,
			feedback: proc.feedback,
			approved: proc.approved,
		}
	})
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
