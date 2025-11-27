package brisa

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration structure for the Brisa server.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
	Middleware MiddlewareConfig `yaml:"middleware"`
}

// ServerConfig holds the server-specific settings like listen address and timeouts.
type ServerConfig struct {
	Listen       string        `yaml:"listen"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// LogConfig holds logging-related settings.
type LogConfig struct {
	Level string `yaml:"level"`
	Path  string `yaml:"path"`
}

// MiddlewareConfig holds the configuration for all middleware chains.
type MiddlewareConfig struct {
	Chains map[string][]MiddlewareInstanceConfig `yaml:"chains"`
}

// MiddlewareInstanceConfig represents a single middleware instance within a chain.
// It includes the type (name) and its specific parameters.
type MiddlewareInstanceConfig map[string]any

// LoadConfig reads configuration data from an io.Reader and unmarshals it
// into a Config struct. It is the caller's responsibility to open and close
// the reader.
func LoadConfig(r io.Reader) (*Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config data: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	return &cfg, nil
}

// LoadConfigFromFile is a helper function that reads a configuration file
// from the given path and unmarshals it into a Config struct.
func LoadConfigFromFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file '%s': %w", path, err)
	}
	defer f.Close()

	cfg, err := LoadConfig(f)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from file '%s': %w", path, err)
	}
	return cfg, nil
}
