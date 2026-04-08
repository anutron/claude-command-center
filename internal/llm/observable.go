package llm

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"
)

// operationKey is the context key for storing the operation name.
type operationKey struct{}

// WithOperation returns a new context with the given operation name attached.
func WithOperation(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, operationKey{}, op)
}

// OperationFrom extracts the operation name from the context.
// Returns empty string if not set.
func OperationFrom(ctx context.Context) string {
	op, _ := ctx.Value(operationKey{}).(string)
	return op
}

// EventPayload carries event data for LLM observability events.
type EventPayload map[string]interface{}

// PublishFunc is a callback that publishes observability events.
type PublishFunc func(topic string, payload EventPayload)

// ObservableLLM wraps an LLM and publishes events for each completion.
type ObservableLLM struct {
	inner   LLM
	publish PublishFunc
	source  string
}

// Compile-time interface check.
var _ LLM = (*ObservableLLM)(nil)

// NewObservableLLM creates an ObservableLLM that wraps inner and publishes
// llm.started / llm.finished events via publish.
func NewObservableLLM(inner LLM, publish PublishFunc, source string) *ObservableLLM {
	return &ObservableLLM{
		inner:   inner,
		publish: publish,
		source:  source,
	}
}

// Complete delegates to the inner LLM and publishes observability events.
func (o *ObservableLLM) Complete(ctx context.Context, prompt string) (string, error) {
	id := generateUUID()

	op := OperationFrom(ctx)
	if op == "" {
		op = "unknown"
	}

	o.publish("llm.started", EventPayload{
		"id":        id,
		"operation": op,
		"source":    o.source,
	})

	start := time.Now()
	result, err := o.inner.Complete(ctx, prompt)
	elapsed := time.Since(start).Milliseconds()

	payload := EventPayload{
		"id":          id,
		"operation":   op,
		"source":      o.source,
		"duration_ms": elapsed,
	}

	if err != nil {
		payload["status"] = "failed"
		payload["error"] = err.Error()
	} else {
		payload["status"] = "completed"
	}

	o.publish("llm.finished", payload)

	return result, err
}

// generateUUID produces a v4 UUID using crypto/rand.
func generateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
