package brisa

import (
	"fmt"
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
