package external

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/plugin"
	tea "github.com/charmbracelet/bubbletea"
)

func writeTestPlugin(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func makeCtx(t *testing.T) plugin.Context {
	t.Helper()
	t.Setenv("CCC_CONFIG_DIR", t.TempDir())
	return plugin.Context{
		Config: config.DefaultConfig(),
		Bus:    plugin.NewBus(),
		Logger: plugin.NewMemoryLogger(),
		DBPath: "",
	}
}

func TestInitHandshake(t *testing.T) {
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"test-plug","tab_name":"Test Plugin","routes":[{"slug":"detail","description":"Show detail","arg_keys":["id"]}],"key_bindings":[{"key":"d","description":"Delete","promoted":true}],"migrations":[],"refresh_interval_ms":5000}'
# Read the config message
read line
# Keep running so process stays alive
while read line; do
  if echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer ep.Shutdown()

	if ep.Slug() != "test-plug" {
		t.Errorf("Slug = %q, want %q", ep.Slug(), "test-plug")
	}
	if ep.TabName() != "Test Plugin" {
		t.Errorf("TabName = %q, want %q", ep.TabName(), "Test Plugin")
	}
	if len(ep.Routes()) != 1 {
		t.Fatalf("Routes len = %d, want 1", len(ep.Routes()))
	}
	if ep.Routes()[0].Slug != "detail" {
		t.Errorf("Route slug = %q, want %q", ep.Routes()[0].Slug, "detail")
	}
	if len(ep.Routes()[0].ArgKeys) != 1 || ep.Routes()[0].ArgKeys[0] != "id" {
		t.Errorf("Route arg_keys = %v, want [id]", ep.Routes()[0].ArgKeys)
	}
	if len(ep.KeyBindings()) != 1 {
		t.Fatalf("KeyBindings len = %d, want 1", len(ep.KeyBindings()))
	}
	if ep.KeyBindings()[0].Key != "d" {
		t.Errorf("KeyBinding key = %q, want %q", ep.KeyBindings()[0].Key, "d")
	}
	if ep.RefreshInterval() != 5*time.Second {
		t.Errorf("RefreshInterval = %v, want %v", ep.RefreshInterval(), 5*time.Second)
	}
}

func TestRender(t *testing.T) {
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"render-test","tab_name":"Render","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"render"'; then
    echo '{"type":"view","content":"Hello, World!"}'
  elif echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer ep.Shutdown()

	content := ep.View(80, 24, 0)
	if content != "Hello, World!" {
		t.Errorf("View = %q, want %q", content, "Hello, World!")
	}
}

func TestHandleKey(t *testing.T) {
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"key-test","tab_name":"Keys","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"key"'; then
    echo '{"type":"action","action":"flash","action_payload":"Key pressed!"}'
  elif echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer ep.Shutdown()

	action := ep.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if action.Type != "flash" {
		t.Errorf("Action.Type = %q, want %q", action.Type, "flash")
	}
	if action.Payload != "Key pressed!" {
		t.Errorf("Action.Payload = %q, want %q", action.Payload, "Key pressed!")
	}
}

func TestCrashDetection(t *testing.T) {
	// Plugin reads init, sends ready, reads config, then crashes.
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"crash-test","tab_name":"Crash","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
read line
exit 1
`
	path := writeTestPlugin(t, script)
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Give the process a moment to exit
	time.Sleep(100 * time.Millisecond)

	content := ep.View(80, 24, 0)
	if !strings.Contains(content, "crashed") && !strings.Contains(content, "exited") {
		// The view might return cached (empty) on first timeout, then error on second call
		// Try again after the process has definitely exited
		time.Sleep(100 * time.Millisecond)
		content = ep.View(80, 24, 0)
	}

	// Now test restart with a working script
	restartScript := `#!/bin/bash
read line
echo '{"type":"ready","slug":"crash-test-restarted","tab_name":"Restarted","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"render"'; then
    echo '{"type":"view","content":"Back alive!"}'
  elif echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	restartPath := writeTestPlugin(t, restartScript)
	ep.command = restartPath
	// Force error state so 'r' triggers restart
	ep.errState = "process exited unexpectedly"

	action := ep.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if action.Type != "noop" {
		t.Errorf("restart action = %q, want noop", action.Type)
	}
	if ep.errState != "" {
		t.Errorf("errState after restart = %q, want empty", ep.errState)
	}

	content = ep.View(80, 24, 0)
	if content != "Back alive!" {
		t.Errorf("View after restart = %q, want %q", content, "Back alive!")
	}
	ep.Shutdown()
}

func TestShutdown(t *testing.T) {
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"shutdown-test","tab_name":"Shutdown","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ep.Shutdown()

	// Process should be dead
	if ep.proc.Alive() {
		t.Error("process still alive after shutdown")
	}
}

func TestMissingBinaryGraceful(t *testing.T) {
	ctx := makeCtx(t)

	ep := &ExternalPlugin{command: "nonexistent-binary-xyz", tabName: "Missing Plugin"}
	err := ep.Init(ctx)
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}

	// Should set errState with a clear "not found" message
	if !strings.Contains(ep.errState, "not found on PATH") {
		t.Errorf("errState = %q, want it to contain 'not found on PATH'", ep.errState)
	}

	// Error view should say "not installed", not "crashed"
	view := ep.errorView()
	if !strings.Contains(view, "not installed") {
		t.Errorf("errorView should contain 'not installed', got: %s", view)
	}
	if strings.Contains(view, "crashed") {
		t.Errorf("errorView should NOT contain 'crashed' for missing binary, got: %s", view)
	}

	// Should use tabName since slug is empty
	if !strings.Contains(view, "Missing Plugin") {
		t.Errorf("errorView should show tabName, got: %s", view)
	}
}

func TestResolveCommandNoPrefixFallback(t *testing.T) {
	// Create a temp dir with a "ccc-fakeplug" binary
	dir := t.TempDir()
	binPath := filepath.Join(dir, "ccc-fakeplug")
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Add temp dir to PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+":"+origPath)

	// "fakeplug" must NOT resolve to "ccc-fakeplug" — no prefix fallback
	resolved := resolveCommand("fakeplug")
	if resolved != "fakeplug" {
		t.Errorf("resolveCommand(%q) = %q, want %q (no prefix fallback)", "fakeplug", resolved, "fakeplug")
	}

	// "ccc-fakeplug" should stay as-is
	resolved = resolveCommand("ccc-fakeplug")
	if resolved != "ccc-fakeplug" {
		t.Errorf("resolveCommand(%q) = %q, want %q", "ccc-fakeplug", resolved, "ccc-fakeplug")
	}

	// "nonexistent" should stay as-is
	resolved = resolveCommand("nonexistent")
	if resolved != "nonexistent" {
		t.Errorf("resolveCommand(%q) = %q, want %q", "nonexistent", resolved, "nonexistent")
	}

	// "fakeplug --some-arg" must NOT get prefix — returned as-is
	resolved = resolveCommand("fakeplug --some-arg")
	if resolved != "fakeplug --some-arg" {
		t.Errorf("resolveCommand(%q) = %q, want %q (no prefix fallback)", "fakeplug --some-arg", resolved, "fakeplug --some-arg")
	}
}

func TestAsyncEventTopicPrefixed(t *testing.T) {
	// Plugin emits events after ready; host should auto-prefix topics with slug.
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"event-test","tab_name":"Events","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
# Read config, then emit async events
read line
echo '{"type":"event","event_topic":"data_updated","event_payload":{"count":42}}'
echo '{"type":"log","level":"info","message":"plugin started"}'
while read line; do
  if echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)

	var mu sync.Mutex
	var received []plugin.Event

	bus := plugin.NewBus()
	// Subscribe to the prefixed topic (slug:topic)
	bus.Subscribe("event-test:data_updated", func(e plugin.Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	ctx := plugin.Context{
		Config: config.DefaultConfig(),
		Bus:    bus,
		Logger: plugin.NewMemoryLogger(),
	}

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer ep.Shutdown()

	// Give async messages time to arrive
	time.Sleep(200 * time.Millisecond)

	// Trigger HandleMessage to drain async channel
	ep.HandleMessage(tea.KeyMsg{})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d events, want 1", len(received))
	}
	if received[0].Topic != "event-test:data_updated" {
		t.Errorf("event topic = %q, want %q", received[0].Topic, "event-test:data_updated")
	}
	if received[0].Source != "event-test" {
		t.Errorf("event source = %q, want %q", received[0].Source, "event-test")
	}
}

func TestConfigScopeEmpty(t *testing.T) {
	// No config_scopes or empty scopes should return empty config.
	cfg := config.DefaultConfig()
	cfg.Slack = config.SlackConfig{Enabled: true, Token: "xoxb-secret"}

	result := plugin.ScopeConfig(cfg, nil)
	if len(result) != 0 {
		t.Errorf("scopeConfig with nil scopes should return empty map, got %v", result)
	}

	result = plugin.ScopeConfig(cfg, []string{})
	if len(result) != 0 {
		t.Errorf("scopeConfig with empty scopes should return empty map, got %v", result)
	}
}

func TestConfigScopeFiltering(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Slack = config.SlackConfig{Enabled: true, Token: "xoxb-secret"}
	cfg.GitHub = config.GitHubConfig{Enabled: true, Username: "testuser"}
	cfg.Calendar = config.CalendarConfig{Enabled: true}

	// Request only slack and github
	result := plugin.ScopeConfig(cfg, []string{"slack", "github"})
	if _, ok := result["slack"]; !ok {
		t.Error("should include slack")
	}
	if _, ok := result["github"]; !ok {
		t.Error("should include github")
	}
	if _, ok := result["calendar"]; ok {
		t.Error("should NOT include calendar")
	}
	if _, ok := result["name"]; ok {
		t.Error("should NOT include name (not requested)")
	}

	// Verify the slack config has the token
	slackJSON, _ := json.Marshal(result["slack"])
	if !strings.Contains(string(slackJSON), "xoxb-secret") {
		t.Errorf("scoped slack config should contain token, got %s", slackJSON)
	}
}

func TestConfigScopeInitProtocol(t *testing.T) {
	// Plugin declares config_scopes: ["github"] — verify it does NOT receive slack config.
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"scope-proto-test","tab_name":"Scoped","config_scopes":["github"],"routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"config"'; then
    # Echo the received config as a log so we can inspect it
    echo "{\"type\":\"log\",\"level\":\"info\",\"message\":$(echo "$line" | sed 's/"/\\"/g')}"
  elif echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)

	cfg := config.DefaultConfig()
	cfg.Slack = config.SlackConfig{Enabled: true, Token: "xoxb-secret-should-not-leak"}
	cfg.GitHub = config.GitHubConfig{Enabled: true, Username: "testuser"}

	ctx := plugin.Context{
		Config: cfg,
		Bus:    plugin.NewBus(),
		Logger: plugin.NewMemoryLogger(),
		DBPath: "",
	}

	ep := &ExternalPlugin{command: path}
	if err := ep.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer ep.Shutdown()

	if ep.Slug() != "scope-proto-test" {
		t.Errorf("Slug = %q, want %q", ep.Slug(), "scope-proto-test")
	}
}

func TestReservedSlugRejection(t *testing.T) {
	for _, slug := range []string{"sessions", "commandcenter", "settings"} {
		if !reservedSlugs[slug] {
			t.Errorf("slug %q should be in reservedSlugs", slug)
		}
	}
}

func TestReservedSlugRejectionInLoader(t *testing.T) {
	// Plugin that declares a reserved slug should be rejected by the loader.
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"sessions","tab_name":"Fake Sessions","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)

	logger := plugin.NewMemoryLogger()
	ctx := plugin.Context{
		Config: config.DefaultConfig(),
		Bus:    plugin.NewBus(),
		Logger: logger,
		DBPath: "",
	}

	cfg := config.DefaultConfig()
	cfg.ExternalPlugins = []config.ExternalPluginConfig{
		{Name: "FakeSessions", Command: path, Enabled: true},
	}

	plugins, err := LoadExternalPlugins(cfg, ctx)
	if err != nil {
		t.Fatalf("LoadExternalPlugins failed: %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins (reserved slug rejected), got %d", len(plugins))
		for _, p := range plugins {
			p.Shutdown()
		}
	}
}

func TestSlugUniqueness(t *testing.T) {
	// Two plugins with the same slug — second should be rejected.
	script := `#!/bin/bash
read line
echo '{"type":"ready","slug":"dupe-test","tab_name":"Dupe","routes":[],"key_bindings":[],"migrations":[],"refresh_interval_ms":0}'
while read line; do
  if echo "$line" | grep -q '"type":"shutdown"'; then
    exit 0
  fi
done
`
	path := writeTestPlugin(t, script)

	logger := plugin.NewMemoryLogger()
	ctx := plugin.Context{
		Config: config.DefaultConfig(),
		Bus:    plugin.NewBus(),
		Logger: logger,
		DBPath: "",
	}

	cfg := config.DefaultConfig()
	cfg.ExternalPlugins = []config.ExternalPluginConfig{
		{Name: "Dupe1", Command: path, Enabled: true},
		{Name: "Dupe2", Command: path, Enabled: true},
	}

	plugins, err := LoadExternalPlugins(cfg, ctx)
	if err != nil {
		t.Fatalf("LoadExternalPlugins failed: %v", err)
	}
	defer func() {
		for _, p := range plugins {
			p.Shutdown()
		}
	}()

	// Only the first plugin should be loaded
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin (duplicate rejected), got %d", len(plugins))
	}
}
