package brisa

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorReader struct{}

func (er *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("forced read error")
}

func TestLoadConfig(t *testing.T) {
	t.Run("successful load", func(t *testing.T) {
		yamlConfig := `
server:
  listen: ":8080"
  read_timeout: 5s
  write_timeout: 10s
log:
  level: "debug"
  path: "/var/log/brisa.log"
middleware:
  chains:
    default:
      - ratelimit:
          requests: 100
          per: 1m
      - cors:
          origins:
            - "https://example.com"
`
		reader := strings.NewReader(yamlConfig)
		cfg, err := LoadConfig(reader)

		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, ":8080", cfg.Server.Listen)
		assert.Equal(t, 5*time.Second, cfg.Server.ReadTimeout)
		assert.Equal(t, 10*time.Second, cfg.Server.WriteTimeout)

		assert.Equal(t, "debug", cfg.Log.Level)
		assert.Equal(t, "/var/log/brisa.log", cfg.Log.Path)

		require.Contains(t, cfg.Middleware.Chains, "default")
		defaultChain := cfg.Middleware.Chains["default"]
		require.Len(t, defaultChain, 2)

		// Check first middleware
		ratelimitConfig := defaultChain[0]
		require.Contains(t, ratelimitConfig, "ratelimit")
		ratelimitParams, ok := ratelimitConfig["ratelimit"].(MiddlewareInstanceConfig)
		require.True(t, ok)
		assert.Equal(t, 100, ratelimitParams["requests"])

		// Check second middleware
		corsConfig := defaultChain[1]
		require.Contains(t, corsConfig, "cors")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		invalidYAML := "server:\n  listen: :8080\n  read_timeout: 5s\n log: level: debug" // bad indentation
		reader := strings.NewReader(invalidYAML)

		cfg, err := LoadConfig(reader)

		assert.Error(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("reader error", func(t *testing.T) {
		reader := &errorReader{}
		cfg, err := LoadConfig(reader)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "forced read error")
		assert.Nil(t, cfg)
	})
}
