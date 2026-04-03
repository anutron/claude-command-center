package main

import (
	"os"
	"strings"
	"testing"
)

func TestMouseCellMotionEnabled(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed to read main.go: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "WithMouseCellMotion") {
		t.Error("main.go must use WithMouseCellMotion to prevent terminal vertical scrolling (text selection via Option+click)")
	}
}
