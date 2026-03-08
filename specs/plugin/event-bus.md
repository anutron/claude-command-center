# SPEC: Event Bus

## Purpose

Provide a typed pub/sub event bus for inter-plugin communication. Plugins publish events and subscribe to topics without direct references to each other.

## Interface

```go
type Event struct {
    Source  string
    Topic   string
    Payload map[string]interface{}
}

type EventBus interface {
    Publish(event Event)
    Subscribe(topic string, handler func(Event))
}
```

## Behavior

- Subscribe registers a handler for a topic. Multiple handlers per topic allowed.
- Publish delivers the event to all handlers subscribed to the event's Topic.
- Events are delivered synchronously in the order handlers were registered.
- Publishing to a topic with no subscribers is a no-op (no error).
- Handlers should not block (long operations should return tea.Cmd via Action).

## Built-in Events

| Topic | Source | Payload | Description |
|-------|--------|---------|-------------|
| project.selected | sessions | path, prompt | User picked a project to launch |
| session.launch | any | dir, args | Request to launch a Claude session |
| todo.created | command-center | title, id | New todo was created |
| todo.completed | command-center | id | Todo marked done |
| todo.dismissed | command-center | id | Todo dismissed |
| focus.updated | command-center | focus | Focus suggestion changed |
| flash | any | message | Display flash message in host |

## Test Cases

- Subscribe then Publish delivers to handler
- Multiple subscribers all receive the event
- Publish with no subscribers does not panic
- Handler receives correct Event fields
- Multiple topics can be subscribed independently
