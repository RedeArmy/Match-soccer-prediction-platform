package config

import (
	"errors"
	"fmt"
	"strings"
)

// knownLogLevels is the set of level strings accepted by the zap logger factory.
// Any value outside this set would cause pkg/logger.New to fall back silently,
// which is harder to diagnose than a startup error.
var knownLogLevels = map[string]struct{}{
	"debug":  {},
	"info":   {},
	"warn":   {},
	"error":  {},
	"dpanic": {},
	"panic":  {},
	"fatal":  {},
}

// knownLogEncodings is the set of encoding strings accepted by the zap logger
// factory. "console" produces human-readable output for local development;
// "json" is required for production log aggregation pipelines.
var knownLogEncodings = map[string]struct{}{
	"json":    {},
	"console": {},
}

// validateWorker enforces the configuration invariants required by the worker
// binary. It is intentionally less strict than validate: the worker has no
// HTTP router (no server.port) and no CORS policy. Infrastructure
// connectivity (database DSN, Redis address) is validated at the point of
// use inside setupDB and setupEventBus.
func validateWorker(cfg *Config) error {
	if _, ok := knownLogLevels[cfg.Logger.Level]; !ok {
		return fmt.Errorf(
			"logger.level %q is not valid (WCQ_LOGGER_LEVEL); accepted values: debug, info, warn, error, dpanic, panic, fatal",
			cfg.Logger.Level,
		)
	}
	if _, ok := knownLogEncodings[cfg.Logger.Encoding]; !ok {
		return fmt.Errorf(
			"logger.encoding %q is not valid (WCQ_LOGGER_ENCODING); accepted values: json, console",
			cfg.Logger.Encoding,
		)
	}
	return validateDatabaseConfig(cfg.Database)
}

// validate enforces the subset of configuration invariants that cannot be
// expressed as safe defaults.
//
// The rule of thumb for what belongs here: if the application cannot start
// safely without the value being present, validate it here so that the
// failure is immediate and the error message names the exact environment
// variable to set. If the application can degrade gracefully without the
// value (e.g. Redis being optional), validate it at the point of use instead.
//
// This function is unexported because it is an implementation detail of
// Load; callers interact only with Load and receive a fully validated Config
// or a descriptive error.
func validate(cfg *Config) error {
	if cfg.Server.Port == "" {
		return errors.New("server.port must not be empty (WCQ_SERVER_PORT)")
	}
	if !cfg.IsDevelopment() {
		if err := validateProductionConfig(cfg); err != nil {
			return err
		}
	}
	if _, ok := knownLogLevels[cfg.Logger.Level]; !ok {
		return fmt.Errorf(
			"logger.level %q is not valid (WCQ_LOGGER_LEVEL); accepted values: debug, info, warn, error, dpanic, panic, fatal",
			cfg.Logger.Level,
		)
	}
	if _, ok := knownLogEncodings[cfg.Logger.Encoding]; !ok {
		return fmt.Errorf(
			"logger.encoding %q is not valid (WCQ_LOGGER_ENCODING); accepted values: json, console",
			cfg.Logger.Encoding,
		)
	}
	return validateDatabaseConfig(cfg.Database)
}

// validateDatabaseConfig enforces pool-sizing invariants that cannot be
// expressed as defaults. It is intentionally separate from the rest of
// validate so it can be called by both validate and validateWorker without
// duplicating logic.
//
// ConnMaxLifetime, ConnMaxIdleTime, and ConnMaxLifetimeJitter are NOT checked
// here because their zero values are meaningful (0 == disabled in pgxpool).
func validateDatabaseConfig(db DatabaseConfig) error {
	if db.MaxOpenConns <= 0 {
		return fmt.Errorf(
			"database.maxOpenConns must be > 0 (WCQ_DATABASE_MAXOPENCONNS); got %d",
			db.MaxOpenConns,
		)
	}
	if db.MaxIdleConns < 0 {
		return fmt.Errorf(
			"database.maxIdleConns must be >= 0 (WCQ_DATABASE_MAXIDLECONNS); got %d",
			db.MaxIdleConns,
		)
	}
	if db.MaxIdleConns > db.MaxOpenConns {
		return fmt.Errorf(
			"database.maxIdleConns (%d) must not exceed database.maxOpenConns (%d); "+
				"set WCQ_DATABASE_MAXIDLECONNS <= WCQ_DATABASE_MAXOPENCONNS",
			db.MaxIdleConns, db.MaxOpenConns,
		)
	}
	return nil
}

// validateProductionConfig enforces invariants that only apply outside
// the development environment (production, staging, etc.).
func validateProductionConfig(cfg *Config) error {
	if cfg.Clerk.JWKSURL == "" {
		return errors.New("clerk.jwksUrl must not be empty outside development (WCQ_CLERK_JWKSURL)")
	}
	if cfg.Clerk.WebhookSecret == "" {
		return errors.New("clerk.webhookSecret must not be empty outside development (WCQ_CLERK_WEBHOOKSECRET)")
	}
	if cfg.Payment.RecurrenteWebhookSecret == "" {
		return errors.New("payment.recurrenteWebhookSecret must not be empty outside development (WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET)")
	}
	if cfg.Payment.PayPalWebhookID == "" {
		return errors.New("payment.paypalWebhookID must not be empty outside development (WCQ_PAYMENT_PAYPALWEBHOOKID)")
	}
	if cfg.Email.UnsubscribeSecret == "" {
		return errors.New("email.unsubscribeSecret must not be empty outside development (WCQ_EMAIL_UNSUBSCRIBESECRET); one-click unsubscribe links will be invalid and the endpoint will return 500")
	}
	if cfg.EventBus.Driver == "in_memory" {
		return fmt.Errorf(
			"eventBus.driver=in_memory is not permitted in production (environment=%q); the in-memory bus cannot deliver events across process boundaries (API → worker). Set WCQ_EVENTBUS_DRIVER=redis",
			cfg.Environment,
		)
	}
	if err := validateCORSOrigins(cfg.CORS.AllowedOrigins, cfg.Environment); err != nil {
		return err
	}
	return validateStorageDriver(cfg.Storage)
}

// validateStorageDriver rejects the local driver in production and delegates
// per-driver field validation to the appropriate helper.
func validateStorageDriver(s StorageConfig) error {
	if s.Driver == "local" {
		return errors.New(
			"storage.driver=local is not permitted in production; files stored on disk are " +
				"lost on pod restart and cannot be shared across replicas. " +
				"Set WCQ_STORAGE_DRIVER to one of: s3, onedrive, gdrive",
		)
	}
	if s.Driver == "s3" {
		return validateS3Config(s)
	}
	if s.Driver == "onedrive" {
		return validateOneDriveConfig(s)
	}
	if s.Driver == "gdrive" {
		return validateGDriveConfig(s)
	}
	return nil
}

func validateS3Config(s StorageConfig) error {
	if s.S3Bucket == "" {
		return errors.New("storage.s3Bucket must not be empty when storage.driver=s3 (WCQ_STORAGE_S3BUCKET)")
	}
	if s.S3Region == "" {
		return errors.New("storage.s3Region must not be empty when storage.driver=s3 (WCQ_STORAGE_S3REGION)")
	}
	return nil
}

func validateOneDriveConfig(s StorageConfig) error {
	if s.OneDriveTenantID == "" {
		return errors.New("storage.onedriveTenantID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVETENANTID)")
	}
	if s.OneDriveClientID == "" {
		return errors.New("storage.onedriveClientID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVECLIENTID)")
	}
	if s.OneDriveClientSecret == "" {
		return errors.New("storage.onedriveClientSecret must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVECLIENTSECRET)")
	}
	if s.OneDriveDriveID == "" {
		return errors.New("storage.onedriveDriveID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVEDRIVEID)")
	}
	return nil
}

// validateCORSOrigins rejects localhost origins in non-development environments.
// An empty allowed-origins list is accepted (it means no CORS — safe for API-only
// deployments behind a same-origin frontend or a gateway that strips the header).
// Listing a localhost origin in production is almost always a misconfiguration:
// browsers enforce same-origin policy correctly, so the real risk is confusion
// during incident response when engineers assume CORS is configured for prod clients.
func validateCORSOrigins(origins []string, env string) error {
	for _, o := range origins {
		if strings.Contains(o, "localhost") || strings.Contains(o, "127.0.0.1") {
			return fmt.Errorf(
				"cors.allowedOrigins contains a localhost origin %q in environment %q; "+
					"localhost origins are only permitted when environment=development. "+
					"Remove it or set WCQ_ENVIRONMENT=development for local runs",
				o, env,
			)
		}
	}
	return nil
}

func validateGDriveConfig(s StorageConfig) error {
	if s.GDriveFolderID == "" {
		return errors.New("storage.gdriveFolderID must not be empty when storage.driver=gdrive (WCQ_STORAGE_GDRIVEFOLDERID)")
	}
	return nil
}
