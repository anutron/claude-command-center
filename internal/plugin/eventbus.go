package plugin

import "sync"

// Event is a typed message passed between plugins via the event bus.
type Event struct {
	Source  string
	Topic   string
	Payload any
}

// EventBus provides pub/sub communication between plugins.
type EventBus interface {
	Publish(event Event)
	Subscribe(topic string, handler func(Event))
}

// Bus is the concrete implementation of EventBus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]func(Event)
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]func(Event)),
	}
}

// Subscribe registers a handler for the given topic.
func (b *Bus) Subscribe(topic string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], handler)
}

// Publish delivers the event to all handlers subscribed to the event's Topic.
func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	handlers := b.handlers[event.Topic]
	b.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}
