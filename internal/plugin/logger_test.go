package plugin

import "testing"

func TestMemoryLogger(t *testing.T) {
	l := NewMemoryLogger()

	l.Info("test-plugin", "hello world")
	l.Warn("test-plugin", "something odd")
	l.Error("test-plugin", "oh no")

	entries := l.Recent(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Level != "INFO" {
		t.Errorf("expected INFO, got %q", entries[0].Level)
	}
	if entries[0].Plugin != "test-plugin" {
		t.Errorf("expected test-plugin, got %q", entries[0].Plugin)
	}
	if entries[0].Message != "hello world" {
		t.Errorf("expected 'hello world', got %q", entries[0].Message)
	}
	if entries[2].Level != "ERROR" {
		t.Errorf("expected ERROR, got %q", entries[2].Level)
	}
}

func TestRecentLimitedCount(t *testing.T) {
	l := NewMemoryLogger()
	for i := 0; i < 10; i++ {
		l.Info("p", "msg")
	}

	entries := l.Recent(3)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestRecentMoreThanAvailable(t *testing.T) {
	l := NewMemoryLogger()
	l.Info("p", "only one")

	entries := l.Recent(100)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestFileLogger(t *testing.T) {
	path := t.TempDir() + "/test.log"
	l, err := NewFileLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.Info("file-plugin", "written to file")

	entries := l.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "written to file" {
		t.Errorf("expected 'written to file', got %q", entries[0].Message)
	}
}
