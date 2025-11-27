package brisa

import (
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/emersion/go-smtp"
)

func TestBrisa_NewSession(t *testing.T) {
	// b := &Brisa{}
	b := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	s, err := b.NewSession(&smtp.Conn{})
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if s == nil {
		t.Errorf("Expected a session, but got nil")
	}
	if _, ok := s.(*Session); !ok {
		t.Errorf("Expected a *Session, but got %T", s)
	}
}

// MockMiddleware is a simple middleware for testing purposes.
func mockMiddlewareFactory(config map[string]any) (*Middleware, error) {
	name := config["name"].(string)
	return &Middleware{
		Handler: func(ctx *Context) Action {
			ctx.logger.Info("mock middleware called", "name", name)
			return Pass
		},
	}, nil
}

func TestNewChainsFromConfig(t *testing.T) {
	// Mock MiddlewareFactory for testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	factory := NewMiddlewareFactory(logger)

	factory.Register("mock-mw", mockMiddlewareFactory)

	factory.Register("error-mw", func(config map[string]any) (*Middleware, error) {
		return nil, errors.New("failed to create")
	})
	factory.Register("panic-mw", func(config map[string]interface{}) (*Middleware, error) {
		panic("middleware creation panicked")
	})

	tests := []struct {
		name          string
		cfg           *MiddlewareConfig
		factory       *MiddlewareFactory
		wantChainsLen map[string]int
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name: "valid config with one middleware",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"type": "mock-mw", "name": "conn-mw-1"},
					},
				},
			},
			factory:       factory,
			wantChainsLen: map[string]int{ChainConn: 1},
			wantErr:       false,
		},
		{
			name: "valid config with multiple middlewares and chains",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"type": "mock-mw", "name": "conn-mw-1"},
						{"type": "mock-mw", "name": "conn-mw-2"},
					},
					ChainMailFrom: {
						{"type": "mock-mw", "name": "mail-from-mw-1"},
					},
				},
			},
			factory:       factory,
			wantChainsLen: map[string]int{ChainConn: 2, ChainMailFrom: 1},
			wantErr:       false,
		},
		{
			name: "config with invalid chain name",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					"invalid-chain": {
						{"type": "mock-mw"},
					},
				},
			},
			factory:    factory,
			wantErr:    true,
			wantErrMsg: "invalid chain name 'invalid-chain' in config",
		},
		{
			name: "middleware config missing type field",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"name": "some-name"}, // 'type' is missing
					},
				},
			},
			factory:    factory,
			wantErr:    true,
			wantErrMsg: "middleware at index 0 in chain 'conn' is missing 'type' field",
		},
		{
			name: "middleware config with empty type field",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"type": ""}, // 'type' is empty
					},
				},
			},
			factory:    factory,
			wantErr:    true,
			wantErrMsg: "middleware at index 0 in chain 'conn' is missing 'type' field",
		},
		{
			name: "factory fails to create middleware",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"type": "error-mw"},
					},
				},
			},
			factory:    factory,
			wantErr:    true,
			wantErrMsg: "failed to create middleware 'error-mw' for chain 'conn': error creating middleware 'error-mw': failed to create",
		},
		{
			name: "factory panics during middleware creation",
			cfg: &MiddlewareConfig{
				Chains: map[string][]MiddlewareInstanceConfig{
					ChainConn: {
						{"type": "panic-mw"},
					},
				},
			},
			factory:    factory,
			wantErr:    true,
			wantErrMsg: "failed to create middleware 'panic-mw' for chain 'conn': panic while creating middleware 'panic-mw': middleware creation panicked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chains, err := NewChainsFromConfig(tt.cfg, tt.factory)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewChainsFromConfig() expected an error, but got nil")
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("NewChainsFromConfig() error = %v, wantErr %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewChainsFromConfig() returned an unexpected error: %v", err)
			}

			if chains == nil {
				t.Fatal("NewChainsFromConfig() returned nil chains")
			}

			if !reflect.DeepEqual(len(chains.chains), len(tt.wantChainsLen)) {
				t.Errorf("Expected %d chains, but got %d", len(tt.wantChainsLen), len(chains.chains))
			}

			for chainName, expectedLen := range tt.wantChainsLen {
				if chain, ok := chains.Get(chainName); !ok || len(chain) != expectedLen {
					t.Errorf("Chain '%s' should have length %d, but got %d (found: %v)", chainName, expectedLen, len(chain), ok)
				}
			}
		})
	}
}
