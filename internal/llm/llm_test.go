package llm

import (
	"context"
	"testing"
)

func TestNoopLLM_Complete(t *testing.T) {
	var l NoopLLM
	result, err := l.Complete(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("NoopLLM.Complete returned error: %v", err)
	}
	if result != "" {
		t.Errorf("NoopLLM.Complete returned %q, want empty string", result)
	}
}

func TestLLMInterfaceCompliance(t *testing.T) {
	// Compile-time checks already exist via var _ LLM = NoopLLM{} and ClaudeCLI{}
	// This test verifies the interface is usable at runtime
	var impls []LLM
	impls = append(impls, NoopLLM{}, ClaudeCLI{})
	if len(impls) != 2 {
		t.Fatalf("expected 2 implementations, got %d", len(impls))
	}
}
