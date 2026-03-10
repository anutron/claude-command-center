# SPEC: Plugin Registry

## Purpose

Manage plugin registration, ordering, and lookup. The registry holds all available plugins and provides slug-based resolution.

## Interface

```go
type Registry struct {
    // unexported fields: plugins []Plugin, bySlug map[string]int
}

func NewRegistry() *Registry
func (r *Registry) Register(p Plugin)
func (r *Registry) All() []Plugin
func (r *Registry) BySlug(slug string) (Plugin, bool)
func (r *Registry) Count() int
func (r *Registry) IndexOf(slug string) int
```

## Behavior

- `Register` adds a plugin to the registry in order of registration
- `All()` returns a copy of plugins in registration order
- `BySlug` looks up a plugin by its `Slug()` value; returns false if not found
- `Count` returns the number of registered plugins
- `IndexOf` returns the index for a plugin slug, or -1 if not found
- Duplicate slugs: last registration wins (overwrites previous at the same index)
- Registry is not concurrent-safe (all registration happens at startup)

## Test Cases

- Register + All returns plugins in order
- BySlug finds registered plugin
- BySlug returns false for unknown slug
- Count returns correct count
- IndexOf returns correct index for registered plugin
- IndexOf returns -1 for unknown slug
- Duplicate slug overwrites previous at same index
