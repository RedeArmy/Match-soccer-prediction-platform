package config

import (
	"time"

	"github.com/spf13/viper"
)

// setDefaults registers a fallback value for every configuration key known
// to the application.
//
// Registering a default — even an explicit empty string — is required for
// viper's AutomaticEnv mechanism to resolve that key from the process
// environment. Keys that have no registered default are silently ignored
// during Unmarshal, which produces a subtle class of bug: the developer
// sets an environment variable, the application ignores it, and the zero
// value of the Go type is used instead. Registering empty-string defaults
// for sensitive keys (DSN, JWT secret) makes this footgun impossible.
//
// Sensitive fields such as database.dsn and jwt.secret intentionally
// default to empty strings. The validation step (validation.go) then
// enforces that they have been supplied at runtime.
func setDefaults(v *viper.Viper) {
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

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.encoding", "json")

	v.SetDefault("jwt.secret", "")
	v.SetDefault("jwt.expiration", 24*time.Hour)

	v.SetDefault("cors.allowedOrigins", "http://localhost:3000")

	v.SetDefault("clerk.jwksUrl", "")
}
