// Package config_test exercises the public surface of pkg/config using
// environment variables as the sole configuration source.
//
// All tests are black-box (package config_test) and interact only through
// Load(), which is the sole public entry point. The internal helpers
// setDefaults and validate are exercised indirectly — testing them in
// isolation would couple the tests to unexported implementation details
// that are free to change without affecting the observable contract of Load.
//
// Environment variables are set and restored per test via t.Setenv, which
// guarantees that each test starts from a clean, reproducible state
// regardless of the order in which the Go test runner executes them.
// Tests that modify the environment must not call t.Parallel(), as parallel
// execution with shared environment state produces non-deterministic results.
package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/config"
)

const (
	fmtUnexpectedError = "unexpected error: %v"
	jwtTestSecret      = "my-secret"
)

// setRequiredEnv configures the minimum set of environment variables for
// which Load has no safe default and would otherwise return a validation
// error. Tests that want to exercise missing or invalid values should call
// t.Setenv to override individual keys after calling this helper.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WCQ_SERVER_PORT", "8080")
	t.Setenv("WCQ_JWT_SECRET", "test-secret-value")
}

func TestLoad_ValidConfig_ReturnsNoError(t *testing.T) {
	setRequiredEnv(t)

	_, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for valid config, got: %v", err)
	}
}

func TestLoad_MissingJWTSecret_ReturnsError(t *testing.T) {
	t.Setenv("WCQ_SERVER_PORT", "8080")
	t.Setenv("WCQ_JWT_SECRET", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for empty JWT secret, got nil")
	}
	if !strings.Contains(err.Error(), "jwt.secret") {
		t.Errorf("expected error message to reference jwt.secret, got: %v", err)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	cases := []struct {
		name string
		got  any
		want any
	}{
		{"Server.ReadTimeout", cfg.Server.ReadTimeout, 10 * time.Second},
		{"Server.WriteTimeout", cfg.Server.WriteTimeout, 30 * time.Second},
		{"Server.IdleTimeout", cfg.Server.IdleTimeout, 60 * time.Second},
		{"Database.MaxOpenConns", cfg.Database.MaxOpenConns, 25},
		{"Database.MaxIdleConns", cfg.Database.MaxIdleConns, 5},
		{"Database.ConnMaxLifetime", cfg.Database.ConnMaxLifetime, 5 * time.Minute},
		{"Redis.Addr", cfg.Redis.Addr, "localhost:6379"},
		{"Redis.DB", cfg.Redis.DB, 0},
		{"Logger.Level", cfg.Logger.Level, "info"},
		{"Logger.Encoding", cfg.Logger.Encoding, "json"},
		{"JWT.Expiration", cfg.JWT.Expiration, 24 * time.Hour},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("default %s: expected %v, got %v", tc.name, tc.want, tc.got)
		}
	}
}

func TestLoad_EnvVarOverridesDefault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_SERVER_PORT", "9090")
	t.Setenv("WCQ_LOGGER_LEVEL", "debug")
	t.Setenv("WCQ_LOGGER_ENCODING", "console")
	t.Setenv("WCQ_DATABASE_MAXOPENCONNS", "50")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	if cfg.Server.Port != "9090" {
		t.Errorf("Server.Port: expected %q, got %q", "9090", cfg.Server.Port)
	}
	if cfg.Logger.Level != "debug" {
		t.Errorf("Logger.Level: expected %q, got %q", "debug", cfg.Logger.Level)
	}
	if cfg.Logger.Encoding != "console" {
		t.Errorf("Logger.Encoding: expected %q, got %q", "console", cfg.Logger.Encoding)
	}
	if cfg.Database.MaxOpenConns != 50 {
		t.Errorf("Database.MaxOpenConns: expected %d, got %d", 50, cfg.Database.MaxOpenConns)
	}
}

func TestLoad_JWTSecretAndPortPopulated(t *testing.T) {
	t.Setenv("WCQ_SERVER_PORT", "3000")
	t.Setenv("WCQ_JWT_SECRET", jwtTestSecret)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	if cfg.Server.Port != "3000" {
		t.Errorf("Server.Port: expected %q, got %q", "3000", cfg.Server.Port)
	}
	if cfg.JWT.Secret != jwtTestSecret {
		t.Errorf("JWT.Secret: expected %q, got %q", jwtTestSecret, cfg.JWT.Secret)
	}
}
