package brisa

import (
	"bytes"
	"fmt"
	"log/slog"
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
			ctx.Status = tc.initialCtxStatus
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

// --- MiddlewareFactory Tests ---

// mockMiddlewareFactoryFunc creates a simple MiddlewareFactoryFunc for testing.
func mockMiddlewareFactoryFunc(returnErr bool) MiddlewareFactoryFunc {
	return func(config map[string]any) (*Middleware, error) {
		if returnErr {
			return nil, fmt.Errorf("factory error")
		}
		return &Middleware{
			Handler:     func(ctx *Context) Action { return Pass },
			IgnoreFlags: Pass, // A non-default value to check if it's overwritten
		}, nil
	}
}

func TestNewMiddlewareFactory(t *testing.T) {
	t.Run("with nil logger", func(t *testing.T) {
		factory := NewMiddlewareFactory(nil)
		if factory == nil {
			t.Fatal("NewMiddlewareFactory returned nil")
		}
		if factory.logger == nil {
			t.Error("Factory logger should not be nil even if input logger is")
		}
	})

	t.Run("with provided logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		factory := NewMiddlewareFactory(logger)
		if factory.logger != logger {
			t.Error("Factory did not use the provided logger")
		}
	})
}

func TestMiddlewareFactory_Register(t *testing.T) {
	factory := NewMiddlewareFactory(nil)

	t.Run("successful registration", func(t *testing.T) {
		err := factory.Register("test-mw", mockMiddlewareFactoryFunc(false))
		if err != nil {
			t.Errorf("Expected no error on first registration, got %v", err)
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		err := factory.Register("test-mw", mockMiddlewareFactoryFunc(false))
		if err == nil {
			t.Error("Expected an error on duplicate registration, got nil")
		}
	})
}

func TestMiddlewareFactory_Unregister(t *testing.T) {
	factory := NewMiddlewareFactory(nil)
	factory.Register("test-mw", mockMiddlewareFactoryFunc(false))

	t.Run("successful unregistration", func(t *testing.T) {
		err := factory.Unregister("test-mw")
		if err != nil {
			t.Errorf("Expected no error on unregistering an existing middleware, got %v", err)
		}
		if _, exists := factory.registry["test-mw"]; exists {
			t.Error("Middleware should have been removed from the registry")
		}
	})

	t.Run("unregister non-existent", func(t *testing.T) {
		err := factory.Unregister("non-existent-mw")
		if err == nil {
			t.Error("Expected an error when unregistering a non-existent middleware, got nil")
		}
	})
}

func TestMiddlewareFactory_Create(t *testing.T) {
	factory := NewMiddlewareFactory(nil)
	factory.Register("success-mw", mockMiddlewareFactoryFunc(false))
	factory.Register("fail-mw", mockMiddlewareFactoryFunc(true))

	testCases := []struct {
		name          string
		mwName        string
		config        map[string]any
		expectErr     bool
		expectedFlags Action
	}{
		{
			name:      "create successful middleware",
			mwName:    "success-mw",
			config:    map[string]any{},
			expectErr: false,
		},
		{
			name:      "create non-existent middleware",
			mwName:    "non-existent-mw",
			config:    map[string]any{},
			expectErr: true,
		},
		{
			name:      "create middleware with factory error",
			mwName:    "fail-mw",
			config:    map[string]any{},
			expectErr: true,
		},
		{
			name:   "create with int ignore_flags",
			mwName: "success-mw",
			config: map[string]any{
				"ignore_flags": int(IgnoreDeliver),
			},
			expectErr:     false,
			expectedFlags: IgnoreDeliver,
		},
		{
			name:   "create with float64 ignore_flags",
			mwName: "success-mw",
			config: map[string]any{
				"ignore_flags": float64(IgnoreQuarantine),
			},
			expectErr:     false,
			expectedFlags: IgnoreQuarantine,
		},
		{
			name:   "create with invalid ignore_flags type",
			mwName: "success-mw",
			config: map[string]any{
				"ignore_flags": "not-a-number",
			},
			expectErr:     false,
			expectedFlags: Pass, // The default from mockMiddlewareFactoryFunc
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mw, err := factory.Create(tc.mwName, tc.config)

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected no error, but got: %v", err)
			}
			if mw == nil {
				t.Fatal("Expected middleware to be created, but it was nil")
			}

			if tc.expectedFlags != 0 && mw.IgnoreFlags != tc.expectedFlags {
				t.Errorf("Expected IgnoreFlags to be %v, but got %v", tc.expectedFlags, mw.IgnoreFlags)
			}
		})
	}
}

func TestMiddlewareFactory_List(t *testing.T) {
	factory := NewMiddlewareFactory(nil)
	factory.Register("mw1", mockMiddlewareFactoryFunc(false))
	factory.Register("mw2", mockMiddlewareFactoryFunc(false))

	list := factory.List()
	if len(list) != 2 {
		t.Fatalf("Expected list of 2 middlewares, got %d", len(list))
	}

	// Order is not guaranteed, so check for presence
	found := make(map[string]bool)
	for _, name := range list {
		found[name] = true
	}
	if !found["mw1"] || !found["mw2"] {
		t.Errorf("List did not contain all registered middlewares. Got: %v", list)
	}
}

func TestMiddlewareChains(t *testing.T) {
	dummyHandler := func(ctx *Context) Action { return Pass }

	t.Run("Register and Get", func(t *testing.T) {
		chains := NewMiddlewareChains()
		chainTypes := []string{
			ChainConn,
			ChainMailFrom,
			ChainRcptTo,
			ChainData,
		}

		// Test registration on all chain types
		for _, chainType := range chainTypes {
			t.Run(string(chainType), func(t *testing.T) {
				// Register a middleware
				mw1 := Middleware{Handler: dummyHandler}
				chains.Register(chainType, mw1)

				// Verify it was registered and defaults were applied
				chain, ok := chains.Get(chainType)
				if !ok {
					t.Fatalf("chain '%s' should exist after registration", chainType)
				}
				if len(chain) != 1 {
					t.Fatalf("expected chain length 1, got %d", len(chain))
				}
				if chain[0].IgnoreFlags != DefaultIgnoreFlags {
					t.Errorf("expected default IgnoreFlags %v, got %v", DefaultIgnoreFlags, chain[0].IgnoreFlags)
				}

				// Register a second middleware with explicit flags
				mw2 := Middleware{Handler: dummyHandler, IgnoreFlags: IgnoreDeliver}
				chains.Register(chainType, mw2)
				chain, _ = chains.Get(chainType)
				if len(chain) != 2 {
					t.Fatalf("expected chain length 2, got %d", len(chain))
				}
				if chain[1].IgnoreFlags != IgnoreDeliver {
					t.Errorf("expected explicit IgnoreFlags %v, got %v", IgnoreDeliver, chain[1].IgnoreFlags)
				}
			})
		}

		t.Run("Get non-existent chain", func(t *testing.T) {
			_, ok := chains.Get("non-existent-chain")
			if ok {
				t.Error("expected 'ok' to be false for a non-existent chain, but it was true")
			}
		})
	})

	t.Run("Default IgnoreFlags application", func(t *testing.T) {
		testCases := []struct {
			name                 string
			middlewareToRegister Middleware
			expectedIgnoreFlags  Action
		}{
			{"when IgnoreFlags is zero, should apply DefaultIgnoreFlags", Middleware{Handler: dummyHandler}, DefaultIgnoreFlags},
			{"when IgnoreFlags is explicitly set, should keep it", Middleware{Handler: dummyHandler, IgnoreFlags: IgnoreDeliver}, IgnoreDeliver},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				chains := NewMiddlewareChains()
				chains.Register(ChainConn, tc.middlewareToRegister)
				chain, _ := chains.Get(ChainConn)

				if len(chain) != 1 {
					t.Fatalf("Expected chain length to be 1, but got %d", len(chain))
				}
				if chain[0].IgnoreFlags != tc.expectedIgnoreFlags {
					t.Errorf("Expected IgnoreFlags to be %v, but got %v", tc.expectedIgnoreFlags, chain[0].IgnoreFlags)
				}
			})
		}
	})
}
