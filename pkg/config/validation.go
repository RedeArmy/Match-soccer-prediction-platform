package config

import (
	"errors"
	"fmt"
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
	return nil
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
		if cfg.EventBus.Driver == "in_memory" {
			return fmt.Errorf(
				"eventBus.driver=in_memory is not permitted in production (environment=%q); the in-memory bus cannot deliver events across process boundaries (API → worker). Set WCQ_EVENTBUS_DRIVER=redis",
				cfg.Environment,
			)
		}
		if cfg.Storage.Driver == "local" {
			return errors.New(
				"storage.driver=local is not permitted in production; files stored on disk are " +
					"lost on pod restart and cannot be shared across replicas. " +
					"Set WCQ_STORAGE_DRIVER to one of: s3, onedrive, gdrive",
			)
		}
		if cfg.Storage.Driver == "s3" {
			if cfg.Storage.S3Bucket == "" {
				return errors.New("storage.s3Bucket must not be empty when storage.driver=s3 (WCQ_STORAGE_S3BUCKET)")
			}
			if cfg.Storage.S3Region == "" {
				return errors.New("storage.s3Region must not be empty when storage.driver=s3 (WCQ_STORAGE_S3REGION)")
			}
		}
		if cfg.Storage.Driver == "onedrive" {
			if cfg.Storage.OneDriveTenantID == "" {
				return errors.New("storage.onedriveTenantID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVETENANTID)")
			}
			if cfg.Storage.OneDriveClientID == "" {
				return errors.New("storage.onedriveClientID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVECLIENTID)")
			}
			if cfg.Storage.OneDriveClientSecret == "" {
				return errors.New("storage.onedriveClientSecret must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVECLIENTSECRET)")
			}
			if cfg.Storage.OneDriveDriveID == "" {
				return errors.New("storage.onedriveDriveID must not be empty when storage.driver=onedrive (WCQ_STORAGE_ONEDRIVEDRIVEID)")
			}
		}
		if cfg.Storage.Driver == "gdrive" {
			if cfg.Storage.GDriveFolderID == "" {
				return errors.New("storage.gdriveFolderID must not be empty when storage.driver=gdrive (WCQ_STORAGE_GDRIVEFOLDERID)")
			}
		}
	}
	if _, ok := knownLogLevels[cfg.Logger.Level]; !ok {
		return fmt.Errorf(
			"logger.level %q is not valid (WCQ_LOGGER_LEVEL); accepted values: debug, info, warn, error, dpanic, panic, fatal",
			cfg.Logger.Level,
		)
	}
	return nil
}
