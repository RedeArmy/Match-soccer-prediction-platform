// Package config defines the application's configuration schema and exposes
// a single Load function that reads values from the environment.
//
// All configuration is resolved once at startup and passed down as concrete
// structs. Reading environment variables at call sites (e.g. os.Getenv inside
// a handler) is explicitly avoided: that pattern hides dependencies, prevents
// upfront validation, and makes tests non-deterministic when the environment
// differs between runs.
package config

import "time"

// Config is the root configuration object for the application.
// It is populated once at startup by Load and treated as immutable thereafter.
// Passing *Config explicitly through the dependency graph — rather than
// accessing a global — makes every component's requirements visible and
// verifiable without running the full application.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	EventBus EventBusConfig `mapstructure:"eventBus"`
	Logger   LoggerConfig   `mapstructure:"logger"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Clerk    ClerkConfig    `mapstructure:"clerk"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

// ServerConfig holds HTTP server tuning parameters.
//
// All timeout values are exposed here rather than hardcoded in the server
// setup because appropriate values differ across environments: a local
// development server can afford generous timeouts, whilst a production
// server behind a load balancer must align with the load balancer's own
// idle and keep-alive settings to avoid silent connection drops.
type ServerConfig struct {
	Port         string        `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"writeTimeout"`
	IdleTimeout  time.Duration `mapstructure:"idleTimeout"`
}

// DatabaseConfig carries the connection string and pool settings for the
// primary PostgreSQL database.
//
// MaxOpenConns and MaxIdleConns are surfaced separately because they serve
// distinct purposes: MaxOpenConns limits the total load placed on the
// database server (protecting it from being overwhelmed), whilst
// MaxIdleConns controls how many connections are retained between traffic
// bursts. Setting MaxIdleConns too high wastes server-side memory; setting
// it too low causes unnecessary connection churn under variable load.
type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"maxOpenConns"`
	MaxIdleConns    int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime time.Duration `mapstructure:"connMaxLifetime"`
}

// RedisConfig carries the address and credentials for the Redis instance
// used for caching and pub/sub messaging.
// Password is optional; many development environments run Redis without
// authentication enabled.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LoggerConfig controls the verbosity and output format of the structured
// logger.
//
// Encoding should be "json" in production so that log aggregation pipelines
// (Datadog, CloudWatch, GCP Logging) can parse fields without fragile regex.
// Use "console" in local development for human-readable, coloured output.
// Never use "console" in production: it is significantly slower and
// incompatible with most log-parsing toolchains.
type LoggerConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
}

// CORSConfig controls which origins, methods, and headers are permitted by
// the CORS middleware.
//
// AllowedOrigins is the list of origins that may make cross-origin requests
// to the API. Set WCQ_CORS_ALLOWEDORIGINS to a comma-separated value to
// supply multiple origins (e.g. "https://app.com,https://staging.app.com").
// In production this must be set to the exact frontend domain. Using a
// wildcard would allow any website to make credentialed requests on behalf
// of a logged-in user, which is effectively a CSRF vulnerability.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowedOrigins"`
}

// EventBusConfig selects which event bus implementation the application uses.
//
// Two drivers are available:
//   - "in_memory": synchronous, in-process delivery. Safe for single-replica
//     deployments and local development. Events are lost on process restart
//     and cannot cross process boundaries.
//   - "redis": asynchronous pub/sub via the Redis instance declared in
//     RedisConfig. Required when running multiple API replicas so that a
//     MatchFinished event published by one replica triggers scoring on all
//     replicas (or on the dedicated worker process).
//
// The default is "in_memory" so that the application starts without a Redis
// dependency in development environments where Redis is not available.
type EventBusConfig struct {
	// Driver must be either "in_memory" or "redis".
	Driver string `mapstructure:"driver"`
}

// WorkerConfig holds tuning parameters for the background worker process.
//
// The worker runs as a separate binary from the API server and exposes a
// lightweight health HTTP server on HealthPort for liveness and readiness
// probes. This port must differ from the API server port (8080) because the
// two processes may be co-located on the same host during canary deployments.
type WorkerConfig struct {
	// HealthPort is the TCP port on which the worker exposes /health and
	// /health/ready. Set WCQ_WORKER_HEALTHPORT to override the default (8081).
	HealthPort string `mapstructure:"healthPort"`
}

// ClerkConfig holds the parameters required to validate JWTs issued by Clerk.
//
// Clerk signs tokens with RS256 using a rotating key pair. The public keys
// are published at the JWKS endpoint and must be fetched and cached at
// startup. The JWKSURL value is available in the Clerk dashboard under
// API Keys → Advanced → JWKS URL.
type ClerkConfig struct {
	JWKSURL string `mapstructure:"jwksUrl"`
	// WebhookSecret is the signing secret from the Clerk webhook dashboard
	// (format "whsec_<base64>"). It is used to validate the Svix signature on
	// incoming webhook events. If empty, signature validation is skipped and a
	// warning is logged — acceptable for local development, never for production.
	WebhookSecret string `mapstructure:"webhookSecret"`
}
