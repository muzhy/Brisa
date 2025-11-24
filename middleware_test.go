package brisa

import (
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
			ctx.Status = tc.initialCtxStatus
			defer FreeContext(ctx)

			finalAction := chain.execute(ctx)

			if finalAction != tc.expectedFinalAction {
				t.Errorf("Expected final action %d, but got %d", tc.expectedFinalAction, finalAction)
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

func TestMiddlewareChains_Register(t *testing.T) {
	// dummyHandler is an empty handler function for testing purposes.
	dummyHandler := func(ctx *Context) Action { return Pass }

	// testCases defines test cases for different registration scenarios.
	testCases := []struct {
		name                 string
		middlewareToRegister Middleware
		expectedIgnoreFlags  Action
	}{
		{
			name:                 "when IgnoreFlags is zero, should apply DefaultIgnoreFlags",
			middlewareToRegister: Middleware{Handler: dummyHandler}, // IgnoreFlags is 0 by default
			expectedIgnoreFlags:  DefaultIgnoreFlags,
		},
		{
			name:                 "when IgnoreFlags is explicitly set to a non-zero value, should keep it",
			middlewareToRegister: Middleware{Handler: dummyHandler, IgnoreFlags: IgnoreDeliver},
			expectedIgnoreFlags:  IgnoreDeliver,
		},
		{
			name:                 "when IgnoreFlags is explicitly set to 0, should apply DefaultIgnoreFlags",
			middlewareToRegister: Middleware{Handler: dummyHandler, IgnoreFlags: 0},
			expectedIgnoreFlags:  DefaultIgnoreFlags,
		},
	}

	// registerFuncs associates the target test methods with the chains they operate on.
	registerFuncs := map[string]struct {
		register func(*middlewareChains, Middleware)
		getChain func(*middlewareChains) *MiddlewareChain
	}{
		"RegisterConnMiddleware": {
			register: func(c *middlewareChains, m Middleware) { c.RegisterConnMiddleware(m) },
			getChain: func(c *middlewareChains) *MiddlewareChain { return &c.ConnChain },
		},
		"RegisterMailFromMiddleware": {
			register: func(c *middlewareChains, m Middleware) { c.RegisterMailFromMiddleware(m) },
			getChain: func(c *middlewareChains) *MiddlewareChain { return &c.MailFromChain },
		},
		"RegisterRcptToMiddleware": {
			register: func(c *middlewareChains, m Middleware) { c.RegisterRcptToMiddleware(m) },
			getChain: func(c *middlewareChains) *MiddlewareChain { return &c.RcptToChain },
		},
		"RegisterDataMiddleware": {
			register: func(c *middlewareChains, m Middleware) { c.RegisterDataMiddleware(m) },
			getChain: func(c *middlewareChains) *MiddlewareChain { return &c.DataChain },
		},
	}

	for funcName, f := range registerFuncs {
		t.Run(funcName, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					chains := NewMiddlewareChains()

					// Register the first middleware and verify
					f.register(chains, tc.middlewareToRegister)
					chain := f.getChain(chains)

					if len(*chain) != 1 {
						t.Fatalf("Expected chain length to be 1, but got %d", len(*chain))
					}
					if (*chain)[0].IgnoreFlags != tc.expectedIgnoreFlags {
						t.Errorf("Expected IgnoreFlags to be %v, but got %v", tc.expectedIgnoreFlags, (*chain)[0].IgnoreFlags)
					}

					// Register a second middleware to verify order and count
					secondMiddleware := Middleware{Handler: dummyHandler, IgnoreFlags: IgnoreDeliver}
					f.register(chains, secondMiddleware)
					if len(*chain) != 2 {
						t.Fatalf("Expected chain length to be 2 after second registration, but got %d", len(*chain))
					}
				})
			}
		})
	}
}
