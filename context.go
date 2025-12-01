package brisa

import (
	"io"
	"log/slog"
	"sync"
)

// Context represents the context of a session.
type Context struct {
	Session *Session
	Logger  *slog.Logger
	Reader  io.Reader
	// Status stores the cumulative status during the execution of the middleware chain.
	Status Action
	keys   map[string]any
	mu     sync.RWMutex
}

// Set stores a new key-value pair in the context.
// It is safe for concurrent use.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.keys == nil {
		c.keys = make(map[string]any)
	}
	c.keys[key] = value
	c.mu.Unlock()
}

// Get returns the value for the given key, and a boolean indicating if the key exists.
// It is safe for concurrent use.
func (c *Context) Get(key string) (value any, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.keys == nil {
		return nil, false
	}
	value, exists = c.keys[key]
	return value, exists
}

// Reset resets the context for reuse.
func (c *Context) Reset() {
	c.Session = nil
	c.Logger = nil
	c.Status = Pass // Reset to the initial state
	c.mu.Lock()
	c.keys = nil
	c.Reader = nil
	c.mu.Unlock()
}

var contextPool = sync.Pool{
	New: func() any {
		return new(Context)
	},
}

// newContext returns a new or recycled Context instance.
func NewContext() *Context {
	c := contextPool.Get().(*Context)
	c.Status = Pass // Ensure the instance from the pool has a clean state
	return c
}

// FreeContext resets and returns a Context instance to the pool.
func FreeContext(c *Context) {
	c.Reset()
	contextPool.Put(c)
}
