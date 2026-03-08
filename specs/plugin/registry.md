# SPEC: Plugin Registry

## Purpose

Manage plugin registration, ordering, and lookup. The registry holds all available plugins and provides slug-based resolution.

## Interface

```go
type Registry struct {
    // unexported fields
}

func NewRegistry() *Registry
func (r *Registry) Register(p Plugin)
func (r *Registry) All() []Plugin
func (r *Registry) BySlug(slug string) (Plugin, bool)
func (r *Registry) Count() int
```

## Behavior

- Register adds a plugin to the registry in order of registration
- All() returns plugins in registration order
- BySlug looks up a plugin by its Slug() value; returns false if not found
- Duplicate slugs: last registration wins (overwrites previous)
- Registry is not concurrent-safe (all registration happens at startup)

## Test Cases

- Register + All returns plugins in order
- BySlug finds registered plugin
- BySlug returns false for unknown slug
- Count returns correct count
- Duplicate slug overwrites previous
