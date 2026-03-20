package automation

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"

	_ "modernc.org/sqlite"
)

// mockScript creates a shell script in tmpDir and returns its path.
func mockScript(t *testing.T, tmpDir, name, content string) string {
	t.Helper()
	path := filepath.Join(tmpDir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}
	return path
}

func TestRunAll_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "good.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"all good"}'
read line
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "test-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Name != "test-auto" {
		t.Errorf("name = %q, want test-auto", r.Name)
	}
	if r.Status != "success" {
		t.Errorf("status = %q, want success", r.Status)
	}
	if r.Message != "all good" {
		t.Errorf("message = %q, want 'all good'", r.Message)
	}
}

func TestRunAll_DisabledAutomation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "disabled-auto",
				Command:  "true",
				Enabled:  false,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "skipped" {
		t.Errorf("status = %q, want skipped", results[0].Status)
	}
	if results[0].Message != "disabled" {
		t.Errorf("message = %q, want disabled", results[0].Message)
	}
}

func TestRunAll_NotDue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "noduerun.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"ran"}'
read line
`)

	now := time.Date(2026, 3, 19, 14, 0, 0, 0, time.Local) // Thursday

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "weekly-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "weekly_friday", // Not Friday
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
		nowFunc: func() time.Time { return now },
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "skipped" {
		t.Errorf("status = %q, want skipped", results[0].Status)
	}
	if results[0].Message != "not due" {
		t.Errorf("message = %q, want 'not due'", results[0].Message)
	}
}

func TestRunAll_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "hang.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
sleep 120
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "hanging-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	results := runner.RunAll(ctx, "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "error" {
		t.Errorf("status = %q, want error", results[0].Status)
	}
}

func TestRunAll_NoAutomations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	runner := &Runner{
		Automations: nil,
		Config:      config.DefaultConfig(),
		DBPath:      dbPath,
		Logger:      plugin.NewMemoryLogger(),
	}

	results := runner.RunAll(context.Background(), "refresh")

	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestRunAll_CommandNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "missing-cmd",
				Command:  "/nonexistent/path/to/automation",
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "error" {
		t.Errorf("status = %q, want error", results[0].Status)
	}
}

func TestRunAll_ScopedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "scoped.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"scoped"}'
read line
`)

	cfg := config.DefaultConfig()
	cfg.Slack = config.SlackConfig{Enabled: true, Token: "xoxb-test"}
	cfg.GitHub = config.GitHubConfig{Enabled: true, Username: "testuser"}

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:         "scoped-auto",
				Command:      script,
				Enabled:      true,
				Schedule:     "every_refresh",
				ConfigScopes: []string{"slack"},
			},
		},
		Config:  cfg,
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "success" {
		t.Errorf("status = %q, want success", results[0].Status)
	}
}

func TestRunAll_LogMessages(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "loggy.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"log","level":"info","message":"step 1"}'
echo '{"type":"log","level":"warn","message":"step 2 warning"}'
echo '{"type":"result","status":"success","message":"done with logs"}'
read line
`)

	logger := plugin.NewMemoryLogger()
	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "loggy-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  logger,
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "success" {
		t.Errorf("status = %q, want success", results[0].Status)
	}

	entries := logger.Recent(10)
	foundInfo := false
	foundWarn := false
	for _, e := range entries {
		if e.Plugin == "automation:loggy-auto" && e.Message == "step 1" {
			foundInfo = true
		}
		if e.Plugin == "automation:loggy-auto" && e.Message == "step 2 warning" {
			foundWarn = true
		}
	}
	if !foundInfo {
		t.Error("expected info log message 'step 1' to be forwarded")
	}
	if !foundWarn {
		t.Error("expected warn log message 'step 2 warning' to be forwarded")
	}
}

func TestRunAll_RecordsToDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "record.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"recorded"}'
read line
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "record-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	runner.RunAll(context.Background(), "refresh")
	runner.RunAll(context.Background(), "refresh")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM cc_automation_runs WHERE name = 'record-auto'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 run records, got %d", count)
	}
}

func TestRunAll_DailySkipsAfterFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "daily.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"daily done"}'
read line
`)

	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.Local)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "daily-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "daily",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
		nowFunc: func() time.Time { return now },
	}

	// First run should execute.
	results := runner.RunAll(context.Background(), "refresh")
	if len(results) != 1 || results[0].Status != "success" {
		t.Fatalf("first run: expected ok, got %v", results)
	}

	// Second run on same day should skip.
	results = runner.RunAll(context.Background(), "refresh")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "skipped" {
		t.Errorf("second run: status = %q, want skipped", results[0].Status)
	}
}

func TestRunAll_ErrorResult(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "errresult.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"error","message":"something broke"}'
read line
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "err-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "error" {
		t.Errorf("status = %q, want error", results[0].Status)
	}
	if results[0].Message != "something broke" {
		t.Errorf("message = %q, want 'something broke'", results[0].Message)
	}
}

func TestRunAll_InitTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script := mockScript(t, tmpDir, "noinit.sh", `#!/bin/sh
read line
sleep 120
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{
				Name:     "noinit-auto",
				Command:  script,
				Enabled:  true,
				Schedule: "every_refresh",
			},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	results := runner.RunAll(ctx, "refresh")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "error" {
		t.Errorf("status = %q, want error", results[0].Status)
	}
}

func TestRunAll_MultipleSequential(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	script1 := mockScript(t, tmpDir, "first.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"first"}'
read line
`)

	script2 := mockScript(t, tmpDir, "second.sh", `#!/bin/sh
read line
echo '{"type":"ready"}'
read line
echo '{"type":"result","status":"success","message":"second"}'
read line
`)

	runner := &Runner{
		Automations: []config.AutomationConfig{
			{Name: "auto-1", Command: script1, Enabled: true, Schedule: "every_refresh"},
			{Name: "auto-2", Command: script2, Enabled: true, Schedule: "every_refresh"},
		},
		Config:  config.DefaultConfig(),
		DBPath:  dbPath,
		Logger:  plugin.NewMemoryLogger(),
		Verbose: true,
	}

	results := runner.RunAll(context.Background(), "refresh")

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "auto-1" || results[0].Status != "success" {
		t.Errorf("result[0] = %+v, want auto-1/ok", results[0])
	}
	if results[1].Name != "auto-2" || results[1].Status != "success" {
		t.Errorf("result[1] = %+v, want auto-2/ok", results[1])
	}
}
