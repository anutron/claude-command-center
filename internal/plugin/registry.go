package plugin

// Registry manages plugin registration, ordering, and lookup.
type Registry struct {
	plugins []Plugin
	bySlug  map[string]int // slug -> index in plugins slice
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		bySlug: make(map[string]int),
	}
}

// Register adds a plugin to the registry.
// If a plugin with the same slug already exists, it is replaced.
func (r *Registry) Register(p Plugin) {
	slug := p.Slug()
	if idx, ok := r.bySlug[slug]; ok {
		r.plugins[idx] = p
		return
	}
	r.bySlug[slug] = len(r.plugins)
	r.plugins = append(r.plugins, p)
}

// All returns all plugins in registration order.
func (r *Registry) All() []Plugin {
	result := make([]Plugin, len(r.plugins))
	copy(result, r.plugins)
	return result
}

// BySlug looks up a plugin by slug.
func (r *Registry) BySlug(slug string) (Plugin, bool) {
	idx, ok := r.bySlug[slug]
	if !ok {
		return nil, false
	}
	return r.plugins[idx], true
}

// Count returns the number of registered plugins.
func (r *Registry) Count() int {
	return len(r.plugins)
}

// IndexOf returns the tab index for a plugin slug, or -1 if not found.
func (r *Registry) IndexOf(slug string) int {
	idx, ok := r.bySlug[slug]
	if !ok {
		return -1
	}
	return idx
}
