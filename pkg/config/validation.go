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
	}
	if _, ok := knownLogLevels[cfg.Logger.Level]; !ok {
		return fmt.Errorf(
			"logger.level %q is not valid (WCQ_LOGGER_LEVEL); accepted values: debug, info, warn, error, dpanic, panic, fatal",
			cfg.Logger.Level,
		)
	}
	return nil
}
