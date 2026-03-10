package plugin

import "testing"

func TestPublishSubscribe(t *testing.T) {
	bus := NewBus()
	var received Event

	bus.Subscribe("test.topic", func(e Event) {
		received = e
	})

	bus.Publish(Event{
		Source:  "test-plugin",
		Topic:   "test.topic",
		Payload: map[string]interface{}{"key": "value"},
	})

	if received.Source != "test-plugin" {
		t.Errorf("expected source 'test-plugin', got %q", received.Source)
	}
	if received.Topic != "test.topic" {
		t.Errorf("expected topic 'test.topic', got %q", received.Topic)
	}
	m, ok := received.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not map[string]interface{}")
	}
	if m["key"] != "value" {
		t.Errorf("expected payload key=value, got %v", m["key"])
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewBus()
	count := 0

	bus.Subscribe("multi", func(e Event) { count++ })
	bus.Subscribe("multi", func(e Event) { count++ })
	bus.Subscribe("multi", func(e Event) { count++ })

	bus.Publish(Event{Topic: "multi"})

	if count != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	bus := NewBus()
	// Should not panic
	bus.Publish(Event{Topic: "nobody-listening"})
}

func TestIndependentTopics(t *testing.T) {
	bus := NewBus()
	topicA := 0
	topicB := 0

	bus.Subscribe("a", func(e Event) { topicA++ })
	bus.Subscribe("b", func(e Event) { topicB++ })

	bus.Publish(Event{Topic: "a"})

	if topicA != 1 {
		t.Errorf("expected topicA=1, got %d", topicA)
	}
	if topicB != 0 {
		t.Errorf("expected topicB=0, got %d", topicB)
	}
}
