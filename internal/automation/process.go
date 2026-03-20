package automation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// process is a lightweight subprocess wrapper for short-lived automations.
// Unlike the external plugin Process, this is synchronous — send a message,
// read a response, repeat.
type process struct {
	cmd    *exec.Cmd
	stdin  *json.Encoder
	stdout *bufio.Scanner
	stderr bytes.Buffer
}

// startProcess spawns a subprocess using "sh -c <command>" with the given context.
func startProcess(ctx context.Context, command string) (*process, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	p := &process{
		cmd:    cmd,
		stdin:  json.NewEncoder(stdinPipe),
		stdout: bufio.NewScanner(stdoutPipe),
	}
	p.stdout.Buffer(make([]byte, 0, 256*1024), 256*1024)

	cmd.Stderr = &p.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	return p, nil
}

// send marshals a HostMsg as a JSON line to the subprocess stdin.
func (p *process) send(msg HostMsg) error {
	return p.stdin.Encode(msg)
}

// receive reads one JSON line from stdout and decodes it as a ResultMsg.
// It blocks until a line is available or the scanner encounters an error/EOF.
func (p *process) receive(timeout time.Duration) (ResultMsg, error) {
	done := make(chan struct{})
	var msg ResultMsg
	var scanErr error

	go func() {
		defer close(done)
		if p.stdout.Scan() {
			scanErr = json.Unmarshal(p.stdout.Bytes(), &msg)
		} else {
			scanErr = p.stdout.Err()
			if scanErr == nil {
				scanErr = fmt.Errorf("subprocess closed stdout")
			}
		}
	}()

	select {
	case <-done:
		return msg, scanErr
	case <-time.After(timeout):
		return ResultMsg{}, fmt.Errorf("timeout after %v", timeout)
	}
}

// wait waits for the subprocess to exit and returns any error.
func (p *process) wait() error {
	return p.cmd.Wait()
}

// kill terminates the subprocess.
func (p *process) kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

// stderrOutput returns captured stderr content, truncated to maxLen bytes.
func (p *process) stderrOutput(maxLen int) string {
	s := p.stderr.String()
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
