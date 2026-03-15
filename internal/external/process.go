package external

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/anutron/claude-command-center/internal/plugin"
	"github.com/anutron/claude-command-center/internal/sanitize"
)

// Process manages the lifecycle of an external plugin subprocess.
type Process struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	mu       sync.Mutex    // protects stdin writes
	syncResp chan PluginMsg // capacity 1, for request/response (view, action)
	asyncCh  chan PluginMsg // capacity 64, for events/logs
	done     chan struct{}  // closed when process exits
	err      error         // set on process exit
	logger   plugin.Logger
	slug     string
}

// Start launches the subprocess using "sh -c <command>".
func (p *Process) Start(command string, logger plugin.Logger) error {
	p.logger = logger
	p.syncResp = make(chan PluginMsg, 1)
	p.asyncCh = make(chan PluginMsg, 64)
	p.done = make(chan struct{})

	p.cmd = exec.Command("sh", "-c", command)
	p.cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	go p.readStdout(stdout)
	go p.readStderr(stderr)

	return nil
}

// Send marshals a HostMsg as JSON + newline and writes it to stdin.
func (p *Process) Send(msg HostMsg) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.done:
		return fmt.Errorf("process exited: %v", p.err)
	default:
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	_, err = p.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// Receive waits for a synchronous response on syncResp with a timeout.
func (p *Process) Receive(timeout time.Duration) (PluginMsg, error) {
	select {
	case msg := <-p.syncResp:
		return msg, nil
	case <-p.done:
		return PluginMsg{}, fmt.Errorf("process exited: %v", p.err)
	case <-time.After(timeout):
		return PluginMsg{}, fmt.Errorf("timeout after %v", timeout)
	}
}

// DrainAsync returns all pending async messages without blocking.
func (p *Process) DrainAsync() []PluginMsg {
	var msgs []PluginMsg
	for {
		select {
		case msg := <-p.asyncCh:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// Alive returns whether the process is still running.
func (p *Process) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Kill terminates the process.
func (p *Process) Kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *Process) readStdout(r io.Reader) {
	defer func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var msg PluginMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			if p.logger != nil {
				p.logger.Warn(p.slug, "invalid JSON from plugin: "+string(line))
			}
			continue
		}

		switch msg.Type {
		case "view", "action", "ready":
			// Synchronous response — non-blocking send to avoid deadlock
			select {
			case p.syncResp <- msg:
			default:
				// Drop if channel full (stale response)
			}
		default:
			select {
			case p.asyncCh <- msg:
			default:
				// Drop if channel full
			}
		}
	}
}

func (p *Process) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := sanitize.StripANSI(scanner.Text())
		if p.logger != nil {
			p.logger.Warn(p.slug, "stderr: "+line)
		}
	}
}
