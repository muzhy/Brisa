package brisa

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// Action represents the action to be taken after a middleware executes. It also
// serves as a status flag. Its values are designed as bit flags to allow for
// bitwise operations with IgnoreFlags.
type Action int // Using int for clear bitwise operations with IgnoreFlags (also an int).

const (
	// Pass continues to the next middleware. It serves as the default/initial state.
	Pass Action = 1 << iota // 1
	// Reject rejects the email and stops processing.
	Reject // 2
	// Deliver marks the email for delivery.
	Deliver // 4
	// Quarantine marks the email for quarantine.
	Quarantine // 8
)

// IgnoreFlags define the statuses that a middleware can ignore.
const (
	// IgnoreDeliver skips the middleware if the context status is Deliver.
	IgnoreDeliver Action = Deliver
	// IgnoreQuarantine skips the middleware if the context status is Quarantine.
	IgnoreQuarantine Action = Quarantine
	// DefaultIgnoreFlags are the default flags for a middleware, causing it to
	// skip execution if the email has already been marked for delivery or quarantine.
	DefaultIgnoreFlags = IgnoreDeliver | IgnoreQuarantine
)

// Handler is the core logic of a middleware, processing the session context.
type Handler func(ctx *Context) Action

// Middleware is a struct containing the handler logic and its metadata.
type Middleware struct {
	// Handler is the function to be executed by this middleware.
	Handler Handler
	// IgnoreFlags is a bitmask indicating which context statuses should cause
	// this middleware to be skipped.
	IgnoreFlags Action
}

// MiddlewareFactoryFunc is the signature for a factory function that creates a middleware.
// It takes a generic map configuration and returns a Middleware instance or an error.
type MiddlewareFactoryFunc func(config map[string]any) (*Middleware, error)

// MiddlewareFactory is a factory for creating and managing middleware.
type MiddlewareFactory struct {
	registry map[string]MiddlewareFactoryFunc
	// mu protects the registry, allowing for dynamic registration of middleware at runtime.
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewMiddlewareFactory creates a new MiddlewareFactory.
func NewMiddlewareFactory(logger *slog.Logger) *MiddlewareFactory {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &MiddlewareFactory{
		registry: make(map[string]MiddlewareFactoryFunc),
		logger:   logger,
	}
}

// Register registers a MiddlewareFactoryFunc with a given name.
// It is safe for concurrent use.
func (f *MiddlewareFactory) Register(name string, factoryFunc MiddlewareFactoryFunc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Disallow re-registering a name.
	if _, exists := f.registry[name]; exists {
		return fmt.Errorf("middleware factory with name '%s' already exists", name)
	}

	f.registry[name] = factoryFunc
	return nil
}

// Unregister removes a registered middleware factory by name.
func (f *MiddlewareFactory) Unregister(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Check if the factory exists before unregistering.
	if _, exists := f.registry[name]; !exists {
		return fmt.Errorf("middleware factory with name '%s' does not exist", name)
	}

	delete(f.registry, name)
	return nil
}

// Create creates a middleware instance by name using its registered factory function.
// It is safe for concurrent use.
// It returns a pointer to allow for post-creation modifications (e.g., setting IgnoreFlags) on the instance.
func (f *MiddlewareFactory) Create(name string, config map[string]any) (*Middleware, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	factoryFunc, exists := f.registry[name]
	if !exists {
		return nil, fmt.Errorf("middleware factory with name '%s' does not exist", name)
	}

	// Call the specific middleware's factory function.
	mw, err := factoryFunc(config)
	if err != nil {
		return nil, fmt.Errorf("error creating middleware '%s': %w", name, err)
	}

	// --- Handle common metadata ---
	// Check for custom ignore_flags in the configuration.
	if flagsVal, ok := config["ignore_flags"]; ok {
		// Try to convert the value to an integer type.
		switch flags := flagsVal.(type) {
		case int:
			mw.IgnoreFlags = Action(flags)
		case float64: // YAML/JSON parsing might result in a float64 for numbers.
			mw.IgnoreFlags = Action(int(flags))
		default:
			f.logger.Warn("Invalid type for 'ignore_flags', expected an integer, using default", "middleware", name, "type", fmt.Sprintf("%T", flagsVal))
		}
	}

	return mw, nil
}

// List returns a slice of names of all registered middleware factories.
// It is safe for concurrent use.
func (f *MiddlewareFactory) List() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	keys := make([]string, 0, len(f.registry))
	for k := range f.registry {
		keys = append(keys, k)
	}
	return keys
}

// MiddlewareChain is a slice of Middleware.
type MiddlewareChain []Middleware

// Execute iterates through and executes all middleware in the chain, passing the
// context to each. It is panic-safe; if a middleware panics, Execute will
// recover, return a Reject action, and an error detailing the panic.
//
// Execution logic:
// - If a middleware's IgnoreFlags match the context's status, it's skipped.
// - The action returned by a handler updates the context's status for subsequent middleware.
// - If a handler returns Reject, execution stops immediately.
func (mc MiddlewareChain) Execute(ctx *Context) (action Action, err error) {
	defer func() {
		if r := recover(); r != nil {
			// A middleware panicked. Recover, set a terminal action, and return an error.
			err = fmt.Errorf("panic recovered during middleware execution: %v", r)
			action = Reject // Reject the session as a safe default.
		}
	}()

	for _, m := range mc {
		// If the context's current status bit overlaps with the middleware's ignore flags, skip this middleware.
		if (m.IgnoreFlags & ctx.Status) != 0 {
			continue
		}

		ctx.Status = m.Handler(ctx)
		if ctx.Status == Reject { // Reject is a terminal state.
			return ctx.Status, nil
		}
	}
	return ctx.Status, nil
}

// MiddlewareChains holds all named middleware chains for the Brisa server.
// It's used to build a complete set of middleware chains that can be atomically
// applied to a Brisa instance. It is not safe for concurrent use; concurrency
// should be managed by the consumer (e.g., Brisa) through atomic replacement
// of the entire instance.
type MiddlewareChains struct {
	chains map[string]MiddlewareChain
}

// NewMiddlewareChains creates a new, empty MiddlewareChains.
func NewMiddlewareChains() *MiddlewareChains {
	return &MiddlewareChains{
		chains: make(map[string]MiddlewareChain),
	}
}

// Register adds a middleware to a specified chain.
// If the chain does not exist, it will be created.
func (c *MiddlewareChains) Register(chainName string, m Middleware) {
	if m.IgnoreFlags == 0 {
		m.IgnoreFlags = DefaultIgnoreFlags // Apply default ignore flags if none are set.
	}
	c.chains[chainName] = append(c.chains[chainName], m)
}

// Get retrieves a middleware chain by its name.
// It returns the chain and a boolean indicating if the chain was found.
// Since MiddlewareChains instances are treated as immutable after creation,
// this method directly returns the internal slice without creating a copy.
func (c *MiddlewareChains) Get(chainName string) (MiddlewareChain, bool) {
	chain, ok := c.chains[chainName]
	return chain, ok
}
