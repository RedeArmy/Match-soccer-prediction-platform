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
//
// WCQ_ENVIRONMENT is set to "dev" explicitly so that tests do not depend on
// the production default and do not need Clerk credentials to pass validation.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envServerPort, "8080")
	t.Setenv(envEnvironment, "dev")
}

// setProductionPaymentEnv sets the payment webhook secrets and the email
// unsubscribe secret required in non-development environments. Tests that
// check unrelated production validations must call this to avoid these checks
// running before the error they intend to observe.
func setProductionPaymentEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET", "test-recurrente-secret")
	t.Setenv("WCQ_PAYMENT_PAYPALWEBHOOKID", "WH-TEST-12345")
	t.Setenv("WCQ_EMAIL_UNSUBSCRIBESECRET", "test-unsubscribe-secret")
}

// setProductionStorageEnv sets the S3 storage variables required in
// non-development environments. The local driver is rejected at startup
// in production; tests that verify other production validations after the
// storage check must call this to prevent an unrelated validation error.
func setProductionStorageEnv(t *testing.T) {
	t.Helper()
	t.Setenv("WCQ_STORAGE_DRIVER", "s3")
	t.Setenv("WCQ_STORAGE_S3BUCKET", "test-bucket")
	t.Setenv("WCQ_STORAGE_S3REGION", "us-east-1")
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
		{"Database.ConnMaxIdleTime", cfg.Database.ConnMaxIdleTime, 10 * time.Minute},
		{"Database.ConnMaxLifetimeJitter", cfg.Database.ConnMaxLifetimeJitter, 30 * time.Second},
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
// Config. Viper resolves its default ("production") when WCQ_ENVIRONMENT is
// absent, so an empty Environment string can only appear via an explicit blank
// override. The method must treat blank as production regardless.
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

func TestLoad_DefaultEnvironmentIsProduction(t *testing.T) {
	// Do NOT call setRequiredEnv here — we are testing the raw viper default
	// before any env override. Production mode requires Clerk credentials, so
	// we supply them to avoid a validation error unrelated to the default under
	// test. server.port has no safe default and must always be provided.
	t.Setenv(envServerPort, "8080")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_testsecret")
	setProductionPaymentEnv(t)
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")
	setProductionStorageEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment default: expected %q, got %q", "production", cfg.Environment)
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

func TestLoad_InvalidLogEncoding_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_LOGGER_ENCODING", "text")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid log encoding, got nil")
	}
	if !strings.Contains(err.Error(), "logger.encoding") {
		t.Errorf("expected error message to reference logger.encoding, got: %v", err)
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
	setProductionPaymentEnv(t)
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")
	setProductionStorageEnv(t)

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for complete production config, got: %v", err)
	}
}

func TestLoad_ProductionWithoutRecurrenteSecret_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	// WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET intentionally not set.
	t.Setenv("WCQ_PAYMENT_PAYPALWEBHOOKID", "WH-TEST-12345")
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing Recurrente webhook secret in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET") {
		t.Errorf("expected error to reference WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET, got: %v", err)
	}
}

func TestLoad_ProductionWithoutPayPalWebhookID_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	t.Setenv("WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET", "test-recurrente-secret")
	// WCQ_PAYMENT_PAYPALWEBHOOKID intentionally not set.
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing PayPal webhook ID in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_PAYMENT_PAYPALWEBHOOKID") {
		t.Errorf("expected error to reference WCQ_PAYMENT_PAYPALWEBHOOKID, got: %v", err)
	}
}

func TestLoad_ProductionWithoutUnsubscribeSecret_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	t.Setenv("WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET", "test-recurrente-secret")
	t.Setenv("WCQ_PAYMENT_PAYPALWEBHOOKID", "WH-TEST-12345")
	// WCQ_EMAIL_UNSUBSCRIBESECRET intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing unsubscribe secret in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_EMAIL_UNSUBSCRIBESECRET") {
		t.Errorf("expected error to reference WCQ_EMAIL_UNSUBSCRIBESECRET, got: %v", err)
	}
}

func TestLoad_ProductionWithInMemoryBus_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	setProductionPaymentEnv(t)
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
	setProductionPaymentEnv(t)
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
	setProductionPaymentEnv(t)
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

func TestLoadWorker_InvalidLogEncoding_ReturnsError(t *testing.T) {
	t.Setenv("WCQ_LOGGER_ENCODING", "text")

	_, err := config.LoadWorker()
	if err == nil {
		t.Fatal("expected error for invalid log encoding, got nil")
	}
	if !strings.Contains(err.Error(), "logger.encoding") {
		t.Errorf("expected error to reference logger.encoding, got: %v", err)
	}
}

// ── CORS ──────────────────────────────────────────────────────────────────────

// TestLoad_CORSDefault_IsEmpty verifies the secure default: no origins are
// allowed by default, forcing operators to explicitly opt in to CORS.
// Updated from the old default of ["http://localhost:3000"] — see DT-25.
func TestLoad_CORSDefault_IsSingleLocalhostOrigin(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Errorf("CORS.AllowedOrigins default: expected empty list, got %v", cfg.CORS.AllowedOrigins)
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

// ── Storage driver validation ─────────────────────────────────────────────────

// setProductionBaseEnv configures all production requirements except storage,
// so storage-driver tests can add only the storage vars they need.
func setProductionBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envServerPort, "8080")
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	setProductionPaymentEnv(t)
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")
}

func TestLoad_ProductionWithLocalStorage_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	// WCQ_STORAGE_DRIVER defaults to "local" — must be rejected in production.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for local storage in production, got nil")
	}
	if !strings.Contains(err.Error(), "storage.driver=local") {
		t.Errorf("expected error to reference storage.driver=local, got: %v", err)
	}
}

func TestLoad_ProductionWithOneDriveStorage_MissingTenantID_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "onedrive")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTID", "client-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTSECRET", "secret")
	t.Setenv("WCQ_STORAGE_ONEDRIVEDRIVEID", "drive-id")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing OneDrive tenant ID in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_ONEDRIVETENANTID") {
		t.Errorf("expected error to reference WCQ_STORAGE_ONEDRIVETENANTID, got: %v", err)
	}
}

func TestLoad_ProductionWithOneDriveStorage_AllFields_ReturnsNoError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "onedrive")
	t.Setenv("WCQ_STORAGE_ONEDRIVETENANTID", "tenant-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTID", "client-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTSECRET", "secret")
	t.Setenv("WCQ_STORAGE_ONEDRIVEDRIVEID", "drive-id")

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for complete OneDrive production config, got: %v", err)
	}
}

func TestLoad_ProductionWithGDriveStorage_MissingFolderID_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "gdrive")
	// WCQ_STORAGE_GDRIVEFOLDERID intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing GDrive folder ID in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_GDRIVEFOLDERID") {
		t.Errorf("expected error to reference WCQ_STORAGE_GDRIVEFOLDERID, got: %v", err)
	}
}

func TestLoad_ProductionWithGDriveStorage_WithFolderID_ReturnsNoError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "gdrive")
	t.Setenv("WCQ_STORAGE_GDRIVEFOLDERID", "folder-id")

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for complete GDrive production config, got: %v", err)
	}
}

func TestLoad_ProductionWithS3Storage_MissingBucket_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "s3")
	t.Setenv("WCQ_STORAGE_S3REGION", "us-east-1")
	// WCQ_STORAGE_S3BUCKET intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing S3 bucket in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_S3BUCKET") {
		t.Errorf("expected error to reference WCQ_STORAGE_S3BUCKET, got: %v", err)
	}
}

func TestLoad_ProductionWithS3Storage_MissingRegion_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "s3")
	t.Setenv("WCQ_STORAGE_S3BUCKET", "test-bucket")
	// WCQ_STORAGE_S3REGION intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing S3 region in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_S3REGION") {
		t.Errorf("expected error to reference WCQ_STORAGE_S3REGION, got: %v", err)
	}
}

func TestLoad_ProductionWithOneDriveStorage_MissingClientID_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "onedrive")
	t.Setenv("WCQ_STORAGE_ONEDRIVETENANTID", "tenant-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTSECRET", "secret")
	t.Setenv("WCQ_STORAGE_ONEDRIVEDRIVEID", "drive-id")
	// WCQ_STORAGE_ONEDRIVECLIENTID intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing OneDrive client ID in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_ONEDRIVECLIENTID") {
		t.Errorf("expected error to reference WCQ_STORAGE_ONEDRIVECLIENTID, got: %v", err)
	}
}

func TestLoad_ProductionWithOneDriveStorage_MissingClientSecret_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "onedrive")
	t.Setenv("WCQ_STORAGE_ONEDRIVETENANTID", "tenant-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTID", "client-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVEDRIVEID", "drive-id")
	// WCQ_STORAGE_ONEDRIVECLIENTSECRET intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing OneDrive client secret in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_ONEDRIVECLIENTSECRET") {
		t.Errorf("expected error to reference WCQ_STORAGE_ONEDRIVECLIENTSECRET, got: %v", err)
	}
}

func TestLoad_ProductionWithOneDriveStorage_MissingDriveID_ReturnsError(t *testing.T) {
	setProductionBaseEnv(t)
	t.Setenv("WCQ_STORAGE_DRIVER", "onedrive")
	t.Setenv("WCQ_STORAGE_ONEDRIVETENANTID", "tenant-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTID", "client-id")
	t.Setenv("WCQ_STORAGE_ONEDRIVECLIENTSECRET", "secret")
	// WCQ_STORAGE_ONEDRIVEDRIVEID intentionally not set.

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing OneDrive drive ID in production, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_STORAGE_ONEDRIVEDRIVEID") {
		t.Errorf("expected error to reference WCQ_STORAGE_ONEDRIVEDRIVEID, got: %v", err)
	}
}

// ── Database pool validation ───────────────────────────────────────────────────

func TestLoad_DatabaseMaxOpenConnsZero_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_DATABASE_MAXOPENCONNS", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for maxOpenConns=0, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_MAXOPENCONNS") {
		t.Errorf("expected error to reference WCQ_DATABASE_MAXOPENCONNS, got: %v", err)
	}
}

func TestLoad_DatabaseMaxIdleConnsExceedsMaxOpenConns_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_DATABASE_MAXOPENCONNS", "5")
	t.Setenv("WCQ_DATABASE_MAXIDLECONNS", "10")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when maxIdleConns > maxOpenConns, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_MAXIDLECONNS") {
		t.Errorf("expected error to reference WCQ_DATABASE_MAXIDLECONNS, got: %v", err)
	}
}

func TestLoad_DatabaseMaxIdleConnsNegative_ReturnsError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_DATABASE_MAXIDLECONNS", "-1")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for negative maxIdleConns, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_MAXIDLECONNS") {
		t.Errorf("expected error to reference WCQ_DATABASE_MAXIDLECONNS, got: %v", err)
	}
}

func TestLoad_DatabasePoolOverride_ConnMaxIdleTime(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_DATABASE_CONNMAXIDLETIME", "5m")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Database.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("Database.ConnMaxIdleTime: expected 5m, got %v", cfg.Database.ConnMaxIdleTime)
	}
}

func TestLoad_DatabasePoolOverride_ConnMaxLifetimeJitter(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("WCQ_DATABASE_CONNMAXLIFETIMEJITTER", "1m")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if cfg.Database.ConnMaxLifetimeJitter != time.Minute {
		t.Errorf("Database.ConnMaxLifetimeJitter: expected 1m, got %v", cfg.Database.ConnMaxLifetimeJitter)
	}
}

// ── CORS default and validation tests ─────────────────────────────────────────

// setProductionCORSBase sets all non-CORS production requirements so that CORS
// validation tests can observe only the CORS-specific error.
func setProductionCORSBase(t *testing.T) {
	t.Helper()
	t.Setenv(envServerPort, "8080")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_testsecret")
	setProductionPaymentEnv(t)
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")
	setProductionStorageEnv(t)
}

// TestLoad_CORSDefault_IsEmpty verifies that the default CORS allowed-origins
// list is empty and does not contain any localhost origins.
func TestLoad_CORSDefault_IsEmpty(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(fmtUnexpectedError, err)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Errorf("CORS default: expected empty list, got %v", cfg.CORS.AllowedOrigins)
	}
}

// TestLoad_CORSLocalhostOrigin_InProduction_ReturnsError verifies that listing
// a localhost origin in a production environment is rejected by validation.
func TestLoad_CORSLocalhostOrigin_InProduction_ReturnsError(t *testing.T) {
	setProductionCORSBase(t)
	t.Setenv("WCQ_CORS_ALLOWEDORIGINS", "http://localhost:3000")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for localhost CORS origin in production, got nil")
	}
	if !strings.Contains(err.Error(), "localhost") {
		t.Errorf("expected error to mention 'localhost', got: %v", err)
	}
}

// TestLoad_CORSLocalhostOrigin_InDevelopment_IsAccepted verifies that localhost
// origins are permitted in the development environment.
func TestLoad_CORSLocalhostOrigin_InDevelopment_IsAccepted(t *testing.T) {
	setRequiredEnv(t) // sets environment=dev
	t.Setenv("WCQ_CORS_ALLOWEDORIGINS", "http://localhost:3000")

	_, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for localhost CORS in development, got: %v", err)
	}
}

// TestLoad_CORSEmptyList_InProduction_IsAccepted verifies that an empty CORS
// list is a valid production configuration (API-only or gateway-terminated deployments).
func TestLoad_CORSEmptyList_InProduction_IsAccepted(t *testing.T) {
	setProductionCORSBase(t)
	// WCQ_CORS_ALLOWEDORIGINS not set — relies on the empty default.

	_, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for empty CORS list in production, got: %v", err)
	}
}

// TestLoad_CORSProductionOrigin_InProduction_IsAccepted verifies that
// non-localhost origins are accepted in production.
func TestLoad_CORSProductionOrigin_InProduction_IsAccepted(t *testing.T) {
	setProductionCORSBase(t)
	t.Setenv("WCQ_CORS_ALLOWEDORIGINS", "https://app.quinielamundial.gt")

	_, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error for production CORS origin, got: %v", err)
	}
}

// ── P3-001: console encoding rejected in production ───────────────────────────

func setFullProductionEnv(t *testing.T) {
	t.Helper()
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "production")
	t.Setenv("WCQ_CLERK_JWKSURL", "https://example.clerk.accounts.dev/.well-known/jwks.json")
	t.Setenv("WCQ_CLERK_WEBHOOKSECRET", "whsec_test")
	setProductionPaymentEnv(t)
	t.Setenv("WCQ_EVENTBUS_DRIVER", "redis")
	setProductionStorageEnv(t)
}

func TestLoad_ProductionWithConsoleEncoding_ReturnsError(t *testing.T) {
	setFullProductionEnv(t)
	t.Setenv("WCQ_LOGGER_ENCODING", "console")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for console encoding in production, got nil")
	}
	if !strings.Contains(err.Error(), "console") {
		t.Errorf("expected error to mention 'console', got: %v", err)
	}
}

func TestLoad_ProductionWithJSONEncoding_ReturnsNoError(t *testing.T) {
	setFullProductionEnv(t)
	t.Setenv("WCQ_LOGGER_ENCODING", "json")

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for json encoding in production, got: %v", err)
	}
}

func TestLoad_DevelopmentWithConsoleEncoding_ReturnsNoError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv(envEnvironment, "dev")
	t.Setenv("WCQ_LOGGER_ENCODING", "console")

	if _, err := config.Load(); err != nil {
		t.Fatalf("expected no error for console encoding in development, got: %v", err)
	}
}

// ── P3-002: ConnMaxLifetime == 0 advisory warning ─────────────────────────────

func TestWarnings_ConnMaxLifetimeZero_ReturnsWarning(t *testing.T) {
	cfg := &config.Config{}
	w := config.Warnings(cfg)
	if len(w) == 0 {
		t.Fatal("expected at least one warning when ConnMaxLifetime is 0, got none")
	}
	found := false
	for _, msg := range w {
		if strings.Contains(msg, "ConnMaxLifetime") || strings.Contains(msg, "connMaxLifetime") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning to mention ConnMaxLifetime, got: %v", w)
	}
}

func TestWarnings_ConnMaxLifetimeNonZero_NoWarning(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.ConnMaxLifetime = 30 * time.Second
	w := config.Warnings(cfg)
	for _, msg := range w {
		if strings.Contains(msg, "ConnMaxLifetime") || strings.Contains(msg, "connMaxLifetime") {
			t.Errorf("unexpected ConnMaxLifetime warning when value is non-zero: %v", msg)
		}
	}
}
