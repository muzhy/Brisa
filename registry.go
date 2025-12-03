package brisa

import "sync"

// MiddlewareFactory defines the function signature for creating a middleware Handler from a config map.
// The config map is typically loaded by the user's application from any source (e.g., YAML, JSON, TOML).
type MiddlewareFactory func(config map[string]any) (Handler, error)

// Registry holds a collection of named middleware factories.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]MiddlewareFactory
}

// NewRegistry creates and returns a new Registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]MiddlewareFactory),
	}
}

// Register adds a new middleware factory with a given name to the registry.
// If a factory with the same name already exists, it will be overwritten.
func (r *Registry) Register(name string, factory MiddlewareFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.factories == nil {
		r.factories = make(map[string]MiddlewareFactory)
	}
	r.factories[name] = factory
}

// Get retrieves a middleware factory by its name.
// It returns the factory and a boolean indicating whether the factory was found.
func (r *Registry) Get(name string) (MiddlewareFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[name]
	return factory, ok
}
