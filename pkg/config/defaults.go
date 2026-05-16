package config

import (
	"time"

	"github.com/spf13/viper"
)

// setDefaults registers a fallback value for every configuration key known
// to the application.
//
// Registering a default - even an explicit empty string - is required for
// viper's AutomaticEnv mechanism to resolve that key from the process
// environment. Keys that have no registered default are silently ignored
// during Unmarshal, which produces a subtle class of bug: the developer
// sets an environment variable, the application ignores it, and the zero
// value of the Go type is used instead. Registering empty-string defaults
// for sensitive keys (DSN, Clerk secrets) makes this footgun impossible.
//
// Sensitive fields such as database.dsn intentionally default to empty
// strings. The validation step (validation.go) then enforces that they
// have been supplied at runtime.
func setDefaults(v *viper.Viper) {
	// environment defaults to "production" so that an unset WCQ_ENVIRONMENT in
	// a deployed container is treated as production (strict auth, redis bus
	// required) rather than silently relaxing all guards. Local developers must
	// explicitly set WCQ_ENVIRONMENT=dev (already present in .env.example).
	v.SetDefault("environment", "production")

	v.SetDefault("server.port", "8080")
	v.SetDefault("server.readTimeout", 10*time.Second)
	v.SetDefault("server.writeTimeout", 30*time.Second)
	v.SetDefault("server.idleTimeout", 60*time.Second)

	v.SetDefault("database.dsn", "")
	v.SetDefault("database.maxOpenConns", 25)
	v.SetDefault("database.maxIdleConns", 5)
	v.SetDefault("database.connMaxLifetime", 5*time.Minute)

	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// eventBus.driver defaults to "in_memory" so the application starts
	// without requiring a Redis connection in development environments.
	// Set WCQ_EVENTBUS_DRIVER=redis to use the Redis-backed bus in production.
	v.SetDefault("eventBus.driver", "in_memory")

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.encoding", "json")

	v.SetDefault("cors.allowedOrigins", []string{"http://localhost:3000"})

	v.SetDefault("clerk.jwksUrl", "")
	v.SetDefault("clerk.webhookSecret", "")

	// payment secrets default to empty; validation.go enforces non-empty values
	// in non-development environments.
	v.SetDefault("payment.recurrenteWebhookSecret", "")
	v.SetDefault("payment.paypalWebhookID", "")

	// worker.healthPort defaults to 8081 so the worker's health endpoints do
	// not collide with the API server (8080) when both run on the same host.
	v.SetDefault("worker.healthPort", "8081")

	// storage.driver defaults to "local" so development environments work
	// without an external object storage service.
	v.SetDefault("storage.driver", "local")
	v.SetDefault("storage.localDir", "uploads")
	// s3
	v.SetDefault("storage.s3Bucket", "")
	v.SetDefault("storage.s3Endpoint", "")
	v.SetDefault("storage.s3Region", "")
	v.SetDefault("storage.s3AccessKeyID", "")
	v.SetDefault("storage.s3SecretKey", "")
	// onedrive
	v.SetDefault("storage.onedriveTenantID", "")
	v.SetDefault("storage.onedriveClientID", "")
	v.SetDefault("storage.onedriveClientSecret", "")
	v.SetDefault("storage.onedriveDriveID", "")
	// gdrive
	v.SetDefault("storage.gdriveCredentialsJSON", "")
	v.SetDefault("storage.gdriveFolderID", "")
}
