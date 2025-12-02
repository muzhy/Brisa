package brisa

import (
	"fmt"
	"testing"
)

// mockHandler creates a simple Handler that returns a specified Action and records whether it was called.
func mockHandler(t *testing.T, name string, returnAction Action, called *bool) Handler {
	return func(ctx *Context) Action {
		t.Logf("Handler '%s' called", name)
		*called = true
		return returnAction
	}
}

// panicHandler creates a handler that immediately panics.
func panicHandler(t *testing.T, name string, called *bool) Handler {
	return func(ctx *Context) Action {
		*called = true
		panic(fmt.Sprintf("panic from %s", name))
	}
}

func TestMiddlewareChain_Execute(t *testing.T) {
	testCases := []struct {
		name                string
		setupMiddlewares    func(t *testing.T, calls map[string]*bool) []Middleware
		initialCtxStatus    Action
		expectedFinalAction Action
		expectedCalls       []string // list of handler names expected to be called
	}{
		{
			name: "No middlewares, should return initial status",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Pass,
			expectedCalls:       []string{},
		},
		{
			name: "Single middleware, default ignore, should execute",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Deliver, calls["m1"])},
				}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Deliver,
			expectedCalls:       []string{"m1"},
		},
		{
			name: "Chain of middlewares, all pass, all should execute",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Pass, calls["m1"])},
					{Handler: mockHandler(t, "m2", Deliver, calls["m2"])},
				}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Deliver,
			expectedCalls:       []string{"m1", "m2"},
		},
		{
			name: "Middleware returns Reject, chain should stop",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Reject, calls["m1"])},
					{Handler: mockHandler(t, "m2", Pass, calls["m2"])},
				}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Reject,
			expectedCalls:       []string{"m1"},
		},
		{
			name: "Context status is Deliver, middleware with IgnoreDeliver should be skipped",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Pass, calls["m1"]), IgnoreFlags: IgnoreDeliver},
					{Handler: mockHandler(t, "m2", Quarantine, calls["m2"])},
				}
			},
			initialCtxStatus:    Deliver,
			expectedFinalAction: Quarantine,
			expectedCalls:       []string{"m2"},
		},
		{
			name: "Context status is Quarantine, middleware with IgnoreDeliver should execute",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Pass, calls["m1"]), IgnoreFlags: IgnoreDeliver},
				}
			},
			initialCtxStatus:    Quarantine,
			expectedFinalAction: Pass,
			expectedCalls:       []string{"m1"},
		},
		{
			name: "Middleware sets status to Deliver, next middleware with IgnoreDeliver is skipped",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Deliver, calls["m1"])},
					{Handler: mockHandler(t, "m2", Pass, calls["m2"]), IgnoreFlags: IgnoreDeliver},
					{Handler: mockHandler(t, "m3", Quarantine, calls["m3"])},
				}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Quarantine,
			expectedCalls:       []string{"m1", "m3"},
		},
		{
			name: "Middleware with multiple ignore flags",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Pass, calls["m1"]), IgnoreFlags: IgnoreDeliver | IgnoreQuarantine},
				}
			},
			initialCtxStatus:    Quarantine,
			expectedFinalAction: Quarantine, // Status doesn't change as middleware is skipped
			expectedCalls:       []string{},
		},
		{
			name: "Middleware panics, should recover and return Reject",
			setupMiddlewares: func(t *testing.T, calls map[string]*bool) []Middleware {
				return []Middleware{
					{Handler: mockHandler(t, "m1", Pass, calls["m1"])},
					{Handler: panicHandler(t, "m2", calls["m2"])},
					{Handler: mockHandler(t, "m3", Pass, calls["m3"])},
				}
			},
			initialCtxStatus:    Pass,
			expectedFinalAction: Reject,
			expectedCalls:       []string{"m1", "m2"}, // m3 should not be called
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup call trackers
			calls := make(map[string]*bool)
			allHandlers := []string{"m1", "m2", "m3"} // A superset of all possible handlers
			for _, name := range allHandlers {
				b := false
				calls[name] = &b
			}

			chain := MiddlewareChain(tc.setupMiddlewares(t, calls))

			ctx := NewContext()
			ctx.Action = tc.initialCtxStatus
			defer FreeContext(ctx)

			finalAction, err := chain.Execute(ctx)

			if finalAction != tc.expectedFinalAction {
				t.Errorf("Expected final action %d, but got %d", tc.expectedFinalAction, finalAction)
			}

			// Check for panicking case
			if tc.name == "Middleware panics, should recover and return Reject" {
				if err == nil {
					t.Error("Expected an error from panic recovery, but got nil")
				}
			}

			// Check which handlers were actually called
			expectedMap := make(map[string]bool)
			for _, name := range tc.expectedCalls {
				expectedMap[name] = true
			}

			for name, ptr := range calls {
				wasCalled := *ptr
				shouldHaveBeenCalled := expectedMap[name]
				if wasCalled != shouldHaveBeenCalled {
					t.Errorf("Handler '%s': expected called=%v, but was %v", name, shouldHaveBeenCalled, wasCalled)
				}
			}
		})
	}
}
