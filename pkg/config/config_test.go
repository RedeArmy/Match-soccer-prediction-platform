// Package config_test exercises the public surface of pkg/config using
// environment variables as the sole configuration source.
//
// All tests are black-box (package config_test) and interact only through
// Load(), which is the sole public entry point. The internal helpers
// setDefaults and validate are exercised indirectly - testing them in
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

	envEnvironment  = "WCQ_ENVIRONMENT"
	envServerPort   = "WCQ_SERVER_PORT"
	envLoggerLevel  = "WCQ_LOGGER_LEVEL"
	portOverride    = "9090"
	portServer      = "3000"
	levelDebug      = "debug"
	encodingConsole = "console"
)

// setRequiredEnv configures the minimum set of environment variables for
// which Load has no safe default and would otherwise return a validation
// error. Tests that want to exercise missing or invalid values should call
// t.Setenv to override individual keys after calling this helper.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envServerPort, "8080")
}

func TestLoad_ValidConfig_ReturnsNoError(t *testing.T) {
	setRequiredEnv(t)

	_, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for valid config, got: %v", err)
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
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("default %s: expected %v, got %v", tc.name, tc.want, tc.got)
		}
	}
}

func TestLoad_EnvVarOverridesDefault(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envServerPort, portOverride)
	t.Setenv(envLoggerLevel, levelDebug)
	t.Setenv("WCQ_LOGGER_ENCODING", encodingConsole)
	t.Setenv("WCQ_DATABASE_MAXOPENCONNS", "50")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	if cfg.Server.Port != portOverride {
		t.Errorf("Server.Port: expected %q, got %q", portOverride, cfg.Server.Port)
	}
	if cfg.Logger.Level != levelDebug {
		t.Errorf("Logger.Level: expected %q, got %q", levelDebug, cfg.Logger.Level)
	}
	if cfg.Logger.Encoding != encodingConsole {
		t.Errorf("Logger.Encoding: expected %q, got %q", encodingConsole, cfg.Logger.Encoding)
	}
	if cfg.Database.MaxOpenConns != 50 {
		t.Errorf("Database.MaxOpenConns: expected %d, got %d", 50, cfg.Database.MaxOpenConns)
	}
}

// TestIsDevelopment_EmptyEnvironment tests the method directly on a zero-value
// Config. Viper resolves its own default ("dev") when WCQ_ENVIRONMENT is absent,
// so the only way to get Environment=="" at runtime is an explicit blank override.
// Regardless, the method must treat blank as production to close the misconfigured-
// container security gap.
func TestIsDevelopment_EmptyEnvironment_IsNotDevelopment(t *testing.T) {
	cfg := &config.Config{Environment: ""}
	if cfg.IsDevelopment() {
		t.Error("IsDevelopment: empty Environment string must not be treated as development")
	}
}

func TestIsDevelopment_ProductionEnvironment_IsNotDevelopment(t *testing.T) {
	for _, env := range []string{"production", "prod", "staging", "unknown"} {
		cfg := &config.Config{Environment: env}
		if cfg.IsDevelopment() {
			t.Errorf("IsDevelopment(%q): want false, got true", env)
		}
	}
}

func TestIsDevelopment_DevelopmentEnvironments_AreRecognised(t *testing.T) {
	for _, env := range []string{"dev", "development", "test"} {
		cfg := &config.Config{Environment: env}
		if !cfg.IsDevelopment() {
			t.Errorf("IsDevelopment(%q): want true, got false", env)
		}
	}
}

func TestLoad_DefaultEnvironmentIsDev(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Environment != "dev" {
		t.Errorf("Environment default: expected %q, got %q", "dev", cfg.Environment)
	}
}

func TestLoad_InvalidLogLevel_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envLoggerLevel, "verbose")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "logger.level") {
		t.Errorf("expected error message to reference logger.level, got: %v", err)
	}
}

func TestLoad_ProductionWithoutJWKSURL_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing JWKS URL in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_CLERK_JWKSURL") {
		t.Errorf("expected error to reference WCQ_CLERK_JWKSURL, got: %v", err)
	}
}

func TestLoad_ProductionWithoutWebhookSecret_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing webhook secret in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_CLERK_WEBHOOKSECRET") {
		t.Errorf("expected error to reference WCQ_CLERK_WEBHOOKSECRET, got: %v", err)
	}
}

func TestLoad_ProductionWithClerkSettings_ReturnsNoError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for complete production config, got: %v", err)
	}
}

func TestLoad_ProductionWithInMemoryBus_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	t.Setenv("WCQ_EVENTBUS_DRIVER", "in_memory")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for in_memory event bus in production, got nil")
	}
	if !strings.Contains(err.Error(), "eventBus.driver=in_memory") {
		t.Errorf("expected error to reference eventBus.driver=in_memory, got: %v", err)
	}
	if !strings.Contains(err.Error(), "WCQ_EVENTBUS_DRIVER=redis") {
		t.Errorf("expected error to suggest WCQ_EVENTBUS_DRIVER=redis, got: %v", err)
	}
}

func TestLoad_ProductionWithDefaultInMemoryBus_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	// Explicitly NOT setting WCQ_EVENTBUS_DRIVER so it defaults to in_memory

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for default in_memory event bus in production, got nil")
	}
	if !strings.Contains(err.Error(), "in_memory") {
		t.Errorf("expected error to reference in_memory, got: %v", err)
	}
}

func TestLoad_StagingWithInMemoryBus_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "staging")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	// in_memory is the default

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for in_memory event bus in staging, got nil")
	}
}

func TestLoad_DevelopmentWithInMemoryBus_ReturnsNoError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "dev")
	// in_memory is the default; this should be allowed in development

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for in_memory event bus in dev, got: %v", err)
	}
	if cfg.EventBus.Driver != "in_memory" {
		t.Errorf("EventBus.Driver: expected %q, got %q", "in_memory", cfg.EventBus.Driver)
	}
}

// ── LoadWorker ────────────────────────────────────────────────────────────────

func TestLoadWorker_ValidConfig_ReturnsNoError(t *testing.T) {
	// LoadWorker does not require WCQ_SERVER_PORT.
	_, err := config.LoadWorker()
	if err != nil {
		t.Fatalf("expected no error for minimal worker config, got: %v", err)
	}
}

func TestLoadWorker_DefaultHealthPort(t *testing.T) {
	cfg, err := config.LoadWorker()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Worker.HealthPort != "8081" {
		t.Errorf("Worker.HealthPort default: expected %q, got %q", "8081", cfg.Worker.HealthPort)
	}
}

func TestLoadWorker_HealthPortOverride(t *testing.T) {
	t.Setenv("WCQ_WORKER_HEALTHPORT", portOverride)

	cfg, err := config.LoadWorker()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Worker.HealthPort != portOverride {
		t.Errorf("Worker.HealthPort override: expected %q, got %q", portOverride, cfg.Worker.HealthPort)
	}
}

func TestLoadWorker_InvalidLogLevel_ReturnsError(t *testing.T) {
	t.Setenv(envLoggerLevel, "verbose")

	_, err := config.LoadWorker()
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "logger.level") {
		t.Errorf("expected error to reference logger.level, got: %v", err)
	}
}

// ── CORS ──────────────────────────────────────────────────────────────────────

func TestLoad_CORSDefault_IsSingleLocalhostOrigin(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	want := []string{"http://localhost:3000"}
	if len(cfg.CORS.AllowedOrigins) != len(want) || cfg.CORS.AllowedOrigins[0] != want[0] {
		t.Errorf("CORS.AllowedOrigins default: expected %v, got %v", want, cfg.CORS.AllowedOrigins)
	}
}

func TestLoad_CORSMultipleOrigins_ParsedFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_CORS_ALLOWEDORIGINS", "https://app.com,https://staging.app.com")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}

	want := []string{"https://app.com", "https://staging.app.com"}
	if len(cfg.CORS.AllowedOrigins) != len(want) {
		t.Fatalf("CORS.AllowedOrigins: expected %d origins, got %d: %v", len(want), len(cfg.CORS.AllowedOrigins), cfg.CORS.AllowedOrigins)
	}
	for i, origin := range want {
		if cfg.CORS.AllowedOrigins[i] != origin {
			t.Errorf("CORS.AllowedOrigins[%d]: expected %q, got %q", i, origin, cfg.CORS.AllowedOrigins[i])
		}
	}
}
