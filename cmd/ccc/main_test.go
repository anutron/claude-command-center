package main

import (
	"os"
	"strings"
	"testing"
)

func TestBUG120_NoMouseModeEnabled(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}
	src := string(data)
	if strings.Contains(src, "WithMouseCellMotion") {
		t.Error("BUG-120 regression: main.go contains WithMouseCellMotion — mouse mode intercepts text selection")
	}
	if strings.Contains(src, "WithMouseAllMotion") {
		t.Error("BUG-120 regression: main.go contains WithMouseAllMotion — mouse mode intercepts text selection")
	}
}
