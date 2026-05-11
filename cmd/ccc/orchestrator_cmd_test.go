package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withOrchestratorTopic sets up a temp orchestrator root and a fake session
// topic file resolving to the supplied name.
func withOrchestratorTopic(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	topics := t.TempDir()
	t.Setenv("CCC_ORCHESTRATOR_ROOT", root)
	t.Setenv("CCC_SESSION_TOPICS_DIR", topics)
	t.Setenv("CCC_SESSION_ID", "test-sess")
	if name != "" {
		if err := os.WriteFile(filepath.Join(topics, "test-sess.txt"), []byte("ORCHESTRATE: "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRunOrchInit_CreatesDirectory(t *testing.T) {
	root := withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init", "--project", "/proj"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "state.md")); err != nil {
		t.Errorf("state.md not created: %v", err)
	}
}

func TestRunOrchInit_FailsWithoutTopic(t *testing.T) {
	withOrchestratorTopic(t, "") // no topic file
	err := runOrchestrator([]string{"init"})
	if err == nil {
		t.Fatal("expected error when topic is missing")
	}
}

func TestRunOrchThreadAdd_RequiresName(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	err := runOrchestrator([]string{"thread", "add"})
	if err == nil {
		t.Fatal("expected --name required")
	}
}

func TestRunOrchFullFlow(t *testing.T) {
	root := withOrchestratorTopic(t, "alpha")

	steps := [][]string{
		{"init", "--project", "/proj"},
		{"thread", "add", "--name", "t1", "--project", "/proj", "--branch", "main", "--status", "in-flight"},
		{"thread", "set-status", "--name", "t1", "--status", "blocked", "--reason", "waiting"},
		{"decision", "add", "--body", "use postgres 16", "--thread", "t1"},
		{"question", "add", "--body", "indexes first?", "--thread", "t1"},
		{"question", "resolve", "--id", "Q1", "--note", "yes, before data"},
		{"thread", "complete", "--name", "t1"},
	}
	for _, args := range steps {
		if err := runOrchestrator(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	state, err := os.ReadFile(filepath.Join(root, "alpha", "state.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"t1", "status: complete", "postgres 16", "Q1", "resolved"} {
		if !strings.Contains(string(state), want) {
			t.Errorf("state.md missing %q\n%s", want, string(state))
		}
	}
	// state.log should contain the intermediate "blocked" transition that
	// was later overwritten by complete.
	logData, err := os.ReadFile(filepath.Join(root, "alpha", "state.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "status=blocked") {
		t.Errorf("state.log missing blocked transition:\n%s", string(logData))
	}
}

func TestRunOrchOverlapCheck_EmitsJSON(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init", "--project", "/Users/aaron/Personal/sherlock"}); err != nil {
		t.Fatal(err)
	}
	// overlap-check writes JSON to stdout — capture by redirecting.
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := runOrchestrator([]string{"overlap-check", "--project", "/Users/aaron/Personal/sherlock"})
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("overlap-check: %v", err)
	}
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, `"name": "alpha"`) {
		t.Errorf("overlap-check did not include alpha: %s", out)
	}
}

func TestRunOrchPasteHeader_FailsForUnknownThread(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	err := runOrchestrator([]string{"paste-header", "--thread", "ghost"})
	if err == nil {
		t.Fatal("expected unknown thread to fail")
	}
}

func TestRunOrchComplete_Idempotent(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrator([]string{"complete"}); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrator([]string{"complete"}); err != nil {
		t.Fatalf("second complete should be no-op success: %v", err)
	}
}

func TestRunOrchUnknownVerb(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	err := runOrchestrator([]string{"frobnicate"})
	if err == nil {
		t.Fatal("expected unknown verb to fail")
	}
}

func TestRunOrchInbox_SendListMarkReadRoundTrip(t *testing.T) {
	root := withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init"}); err != nil {
		t.Fatal(err)
	}

	// Send a handoff and a checkin.
	if err := runOrchestrator([]string{"inbox", "send", "--to", "a", "--kind", "handoff", "--body", "do X"}); err != nil {
		t.Fatalf("send handoff: %v", err)
	}
	if err := runOrchestrator([]string{"inbox", "send", "--to", "orchestrator", "--from", "a", "--kind", "checkin", "--body", "starting"}); err != nil {
		t.Fatalf("send checkin: %v", err)
	}

	// inbox.jsonl should have exactly two lines.
	data, err := os.ReadFile(filepath.Join(root, "alpha", "inbox.jsonl"))
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	lines := strings.Count(strings.TrimRight(string(data), "\n"), "\n") + 1
	if lines != 2 {
		t.Errorf("expected 2 inbox lines, got %d:\n%s", lines, string(data))
	}

	// list --unread --to orchestrator should include the checkin (id 2).
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	if err := runOrchestrator([]string{"inbox", "list", "--unread", "--to", "orchestrator", "--json"}); err != nil {
		t.Fatalf("list unread: %v", err)
	}
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, `"kind": "checkin"`) {
		t.Errorf("expected checkin in unread output, got: %s", out)
	}
	if strings.Contains(out, `"kind": "handoff"`) {
		t.Errorf("handoff (to a, not orchestrator) leaked into orchestrator's inbox: %s", out)
	}

	// mark-read advances cursor; subsequent unread query returns empty array.
	if err := runOrchestrator([]string{"inbox", "mark-read", "--to", "orchestrator"}); err != nil {
		t.Fatalf("mark-read: %v", err)
	}
	r, w, _ = os.Pipe()
	old = os.Stdout
	os.Stdout = w
	if err := runOrchestrator([]string{"inbox", "list", "--unread", "--to", "orchestrator", "--json"}); err != nil {
		t.Fatalf("list unread after mark-read: %v", err)
	}
	w.Close()
	os.Stdout = old
	n, _ = r.Read(buf)
	out = string(buf[:n])
	if !strings.Contains(out, "[]") {
		t.Errorf("expected empty unread after mark-read, got: %s", out)
	}
}

func TestRunOrchInbox_SendRequiresFields(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{"inbox", "send", "--kind", "handoff", "--body", "x"},        // missing --to
		{"inbox", "send", "--to", "a", "--body", "x"},                // missing --kind
		{"inbox", "send", "--to", "a", "--kind", "handoff"},          // missing --body
	}
	for _, args := range cases {
		if err := runOrchestrator(args); err == nil {
			t.Errorf("%v: expected failure", args)
		}
	}
}

func TestRunOrchInbox_ResolveRoleMatchesWorktree(t *testing.T) {
	withOrchestratorTopic(t, "alpha")
	if err := runOrchestrator([]string{"init", "--project", "/proj"}); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrator([]string{"thread", "add", "--name", "a", "--project", "/proj", "--worktree", "/proj/.wt/a"}); err != nil {
		t.Fatal(err)
	}
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	if err := runOrchestrator([]string{"inbox", "resolve-role", "--worktree", "/proj/.wt/a"}); err != nil {
		t.Fatalf("resolve-role: %v", err)
	}
	w.Close()
	os.Stdout = old
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if got := strings.TrimSpace(string(buf[:n])); got != "alpha:a" {
		t.Errorf("expected alpha:a, got %q", got)
	}
}
