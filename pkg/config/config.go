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
	Logger   LoggerConfig   `mapstructure:"logger"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Clerk    ClerkConfig    `mapstructure:"clerk"`
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

// JWTConfig holds the parameters for issuing and validating JSON Web Tokens.
//
// Secret must be a cryptographically random string of at least 32 bytes.
// It must never be committed to version control or embedded in a Docker image.
// Always inject it at runtime via the WCQ_JWT_SECRET environment variable,
// sourced from a secrets manager such as AWS Secrets Manager or HashiCorp Vault.
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Expiration time.Duration `mapstructure:"expiration"`
}

// CORSConfig controls which origins, methods, and headers are permitted by
// the CORS middleware.
//
// AllowedOrigins is a comma-separated list of origins that may make
// cross-origin requests to the API. In production this must be set to the
// exact frontend domain (e.g. "https://myapp.com"). Using a wildcard in
// production would allow any website to make credentialed requests on behalf
// of a logged-in user, which is effectively a CSRF vulnerability.
type CORSConfig struct {
	AllowedOrigins string `mapstructure:"allowedOrigins"`
}

// ClerkConfig holds the parameters required to validate JWTs issued by Clerk.
//
// Clerk signs tokens with RS256 using a rotating key pair. The public keys
// are published at the JWKS endpoint and must be fetched and cached at
// startup. The JWKSURL value is available in the Clerk dashboard under
// API Keys → Advanced → JWKS URL.
type ClerkConfig struct {
	JWKSURL       string `mapstructure:"jwksUrl"`
	// WebhookSecret is the signing secret from the Clerk webhook dashboard
	// (format "whsec_<base64>"). It is used to validate the Svix signature on
	// incoming webhook events. If empty, signature validation is skipped and a
	// warning is logged — acceptable for local development, never for production.
	WebhookSecret string `mapstructure:"webhookSecret"`
}
