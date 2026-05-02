package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// LoadWorker resolves the configuration for the background worker binary.
//
// It uses the same environment variable convention as Load (WCQ_ prefix, dot
// notation mapped to underscores) but applies validateWorker instead of
// validate, skipping the API-only requirement of server.port.
func LoadWorker() (*Config, error) {
	v := viper.New()

	v.SetEnvPrefix("WCQ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validateWorker(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Load resolves the application configuration in the following precedence
// order (highest to lowest): environment variables -> registered defaults.
//
// Every environment variable must be prefixed with WCQ_ and use underscores
// to separate nested keys. For example, the configuration key "database.dsn"
// is read from the environment variable WCQ_DATABASE_DSN. The prefix
// prevents collisions with variables injected by the container runtime,
// CI pipelines, or the operating system itself.
//
// A fresh viper instance is created on every call rather than using the
// package-level global (viper.SetDefault, viper.AutomaticEnv, etc.). This
// is deliberate: the global singleton accumulates state across calls, which
// makes parallel tests non-deterministic. Owning the instance explicitly
// keeps Load idempotent and safe to call multiple times in tests.
func Load() (*Config, error) {
	v := viper.New()

	v.SetEnvPrefix("WCQ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}
