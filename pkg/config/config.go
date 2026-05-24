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
// Passing *Config explicitly through the dependency graph - rather than
// accessing a global - makes every component's requirements visible and
// verifiable without running the full application.
type Config struct {
	Environment string         `mapstructure:"environment"`
	Server      ServerConfig   `mapstructure:"server"`
	Database    DatabaseConfig `mapstructure:"database"`
	Redis       RedisConfig    `mapstructure:"redis"`
	EventBus    EventBusConfig `mapstructure:"eventBus"`
	Logger      LoggerConfig   `mapstructure:"logger"`
	CORS        CORSConfig     `mapstructure:"cors"`
	Clerk       ClerkConfig    `mapstructure:"clerk"`
	Payment     PaymentConfig  `mapstructure:"payment"`
	Worker      WorkerConfig   `mapstructure:"worker"`
	Storage     StorageConfig  `mapstructure:"storage"`
	Tracing     TracingConfig  `mapstructure:"tracing"`
	Metrics     MetricsConfig  `mapstructure:"metrics"`
	Email       EmailConfig    `mapstructure:"email"`
	N8n         N8nConfig      `mapstructure:"n8n"`
	WebPush     WebPushConfig  `mapstructure:"webPush"`
}

// EmailConfig holds credentials for the transactional email provider (Resend).
//
// ResendAPIKey is the only required field.  An empty key results in a NoopClient
// being used — emails are silently discarded, which is appropriate for local
// development but will cause missed alerts in production.
//
// Set via WCQ_EMAIL_RESENDAPIKEY and WCQ_EMAIL_FROMADDRESS.
type EmailConfig struct {
	// ResendAPIKey is the Resend API key.  Keep this out of source control;
	// inject via environment variable or secrets manager.
	ResendAPIKey string `mapstructure:"resendAPIKey"`
	// FromAddress is the sender address stamped on every outgoing email.
	// Example: "World Cup Quiniela <noreply@quinielamundial.gt>".
	FromAddress string `mapstructure:"fromAddress"`
	// UnsubscribeSecret is the HMAC-SHA256 secret used to sign and verify
	// email unsubscribe tokens (CAN-SPAM / GDPR compliance).
	// An empty value disables the unsubscribe link in outgoing emails and
	// the /api/v1/notifications/unsubscribe endpoint returns 500.
	// Set via WCQ_EMAIL_UNSUBSCRIBESECRET.
	UnsubscribeSecret string `mapstructure:"unsubscribeSecret"`
}

// N8nConfig holds the configuration for the n8n automation platform.
//
// The config splits concerns into two layers:
//   - AdminDispatcher (system.* outbox events) uses WebhookURL as a single
//     endpoint for dispatching admin notifications.
//   - ObservabilityNotifier uses BaseURL to construct per-event webhook paths
//     (e.g. BaseURL + "/webhook/dlq-overflow") for operational alerting.
//
// Both layers sign requests with WebhookSecret when set.
type N8nConfig struct {
	// WebhookURL is the full n8n webhook endpoint that AdminDispatcher uses for
	// system.* outbox events. An empty value disables the admin dispatcher
	// integration. Set via WCQ_N8N_WEBHOOKURL.
	WebhookURL string `mapstructure:"webhookURL"`
	// WebhookSecret is the shared HMAC-SHA256 signing key stamped in the
	// X-Signature header of all outgoing n8n requests. An empty value sends
	// unsigned requests and logs a startup warning.
	// Set via WCQ_N8N_WEBHOOKSECRET.
	WebhookSecret string `mapstructure:"webhookSecret"`
	// BaseURL is the n8n base URL consumed by ObservabilityNotifier to
	// construct specific webhook paths per event type (e.g. "http://n8n:5678").
	// When empty, all observability notifications are disabled.
	// Set via WCQ_N8N_BASEURL.
	BaseURL string `mapstructure:"baseURL"`
	// APIKey is the n8n API key used to call n8n's REST API for the admin
	// observability endpoints under /api/admin/observability/n8n/*.
	// Set via WCQ_N8N_APIKEY.
	APIKey string `mapstructure:"apiKey"`
}

// WebPushConfig holds secrets required for RFC 8292 Web Push via VAPID.
//
// VAPIDPrivateKey is a cryptographic secret and must never be stored in the
// database or configuration files. Inject it via WCQ_WEBPUSH_VAPIDPRIVATEKEY.
// The public key and subject are non-secret and live in system_params.
type WebPushConfig struct {
	// VAPIDPrivateKey is the base64url-encoded P-256 private key used to sign
	// VAPID JWT assertions (RFC 8292). An empty value disables Web Push.
	// Set via WCQ_WEBPUSH_VAPIDPRIVATEKEY.
	VAPIDPrivateKey string `mapstructure:"vapidPrivateKey"`
}

// StorageConfig configures the object storage backend used for binary assets
// such as bank transfer proof images.
//
// Four drivers are available:
//   - "local"    — filesystem, development only. Lost on pod restart.
//   - "s3"       — AWS S3, Cloudflare R2, or any S3-compatible service.
//   - "onedrive" — Microsoft OneDrive / SharePoint via the Graph API.
//   - "gdrive"   — Google Drive via a service account or ADC.
//
// Only the fields relevant to the selected driver need to be set; all others
// are silently ignored.
type StorageConfig struct {
	// Driver selects the backing implementation.
	// Defaults to "local". Production deployments must use one of: s3, onedrive, gdrive.
	Driver string `mapstructure:"driver"`

	// ── local ────────────────────────────────────────────────────────────────

	// LocalDir is the filesystem root used by the "local" driver.
	// Ignored when Driver is not "local".
	LocalDir string `mapstructure:"localDir"`

	// ── s3 ───────────────────────────────────────────────────────────────────

	// S3Bucket is the bucket name. Required when Driver is "s3".
	// Set via WCQ_STORAGE_S3BUCKET.
	S3Bucket string `mapstructure:"s3Bucket"`
	// S3Endpoint overrides the default AWS endpoint (Cloudflare R2, MinIO).
	// Set via WCQ_STORAGE_S3ENDPOINT.
	S3Endpoint string `mapstructure:"s3Endpoint"`
	// S3Region is the AWS region or "auto" for Cloudflare R2. Required when
	// Driver is "s3". Set via WCQ_STORAGE_S3REGION.
	S3Region string `mapstructure:"s3Region"`
	// S3AccessKeyID and S3SecretKey provide static credentials. When both are
	// empty the SDK falls back to the standard credential chain.
	// Set via WCQ_STORAGE_S3ACCESSKEYID and WCQ_STORAGE_S3SECRETKEY.
	S3AccessKeyID string `mapstructure:"s3AccessKeyID"`
	S3SecretKey   string `mapstructure:"s3SecretKey"`

	// ── onedrive ─────────────────────────────────────────────────────────────

	// OneDriveTenantID is the Azure AD tenant ID (GUID or verified domain).
	// Required when Driver is "onedrive". Set via WCQ_STORAGE_ONEDRIVETENANTID.
	OneDriveTenantID string `mapstructure:"onedriveTenantID"`
	// OneDriveClientID is the Azure application (client) ID.
	// Required when Driver is "onedrive". Set via WCQ_STORAGE_ONEDRIVECLIENTID.
	OneDriveClientID string `mapstructure:"onedriveClientID"`
	// OneDriveClientSecret is the Azure application client secret.
	// Required when Driver is "onedrive". Set via WCQ_STORAGE_ONEDRIVECLIENTSECRET.
	OneDriveClientSecret string `mapstructure:"onedriveClientSecret"`
	// OneDriveDriveID is the Microsoft Graph drive ID (e.g. a SharePoint drive
	// or the value from GET /me/drive).
	// Required when Driver is "onedrive". Set via WCQ_STORAGE_ONEDRIVEDRIVEID.
	OneDriveDriveID string `mapstructure:"onedriveDriveID"`

	// ── gdrive ───────────────────────────────────────────────────────────────

	// GDriveCredentialsJSON is the raw JSON of a Google service-account key.
	// When empty, Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS
	// or GCE metadata) are used instead.
	// Set via WCQ_STORAGE_GDRIVECREDENTIALSJSON.
	GDriveCredentialsJSON string `mapstructure:"gdriveCredentialsJSON"`
	// GDriveFolderID is the Google Drive folder that acts as the storage root.
	// Required when Driver is "gdrive". Set via WCQ_STORAGE_GDRIVEFOLDERID.
	GDriveFolderID string `mapstructure:"gdriveFolderID"`
}

// IsDevelopment reports whether the application is running in a relaxed local
// environment where auth/webhook secrets may be omitted intentionally.
// An empty Environment is treated as production - an unset WCQ_ENVIRONMENT
// in a deployed container is a misconfiguration, not implicit development mode.
// The viper default ("dev") ensures a developer who never sets the variable
// still gets development mode; only an explicit empty string is rejected.
func (c *Config) IsDevelopment() bool {
	switch c.Environment {
	case "dev", "development", "test":
		return true
	default:
		return false
	}
}

// ServerConfig holds HTTP server tuning parameters.
//
// All timeout values are exposed here rather than hardcoded in the server
// setup because appropriate values differ across environments: a local
// development server can afford generous timeouts, whilst a production
// server behind a load balancer must align with the load balancer's own
// idle and keep-alive settings to avoid silent connection drops.
type ServerConfig struct {
	Port            string        `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"readTimeout"`
	WriteTimeout    time.Duration `mapstructure:"writeTimeout"`
	IdleTimeout     time.Duration `mapstructure:"idleTimeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdownTimeout"`
	// AppBaseURL is the externally reachable base URL of this service
	// (e.g. "https://app.quinielamundial.gt"), used to build absolute links
	// inside outgoing emails.  No trailing slash.
	// Set via WCQ_SERVER_APPBASEURL.
	AppBaseURL string `mapstructure:"appBaseURL"`
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
//
// ConnMaxIdleTime evicts connections that have been idle longer than the
// specified duration. Zero disables idle eviction (pgxpool holds them until
// ConnMaxLifetime expires or the server closes them). A non-zero value
// is recommended for services that see bursty traffic.
//
// ConnMaxLifetimeJitter adds a random offset (0..jitter) to each
// connection's max-lifetime, preventing the pool from recycling all
// connections at the same instant after a deployment.
type DatabaseConfig struct {
	DSN                   string        `mapstructure:"dsn"`
	MaxOpenConns          int           `mapstructure:"maxOpenConns"`
	MaxIdleConns          int           `mapstructure:"maxIdleConns"`
	ConnMaxLifetime       time.Duration `mapstructure:"connMaxLifetime"`
	ConnMaxIdleTime       time.Duration `mapstructure:"connMaxIdleTime"`
	ConnMaxLifetimeJitter time.Duration `mapstructure:"connMaxLifetimeJitter"`
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
//     and cannot cross process boundaries. Rejected by config validation
//     outside development environments.
//   - "redis": asynchronous delivery via Redis Streams. Required for
//     multi-replica API deployments and for the separate worker process.
//     Set WCQ_EVENTBUS_DRIVER=redis in all non-local environments.
//
// The effective default in production (WCQ_ENVIRONMENT unset or set to
// anything other than dev/development/test) is "redis" because config
// validation rejects "in_memory" outside development. Local developers must
// set WCQ_ENVIRONMENT=dev (already present in .env.example) to use
// "in_memory" without a Redis dependency.
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
// API Keys -> Advanced -> JWKS URL.
type ClerkConfig struct {
	JWKSURL string `mapstructure:"jwksUrl"`
	// WebhookSecret is the signing secret from the Clerk webhook dashboard
	// (format "whsec_<base64>"). It is used to validate the Svix signature on
	// incoming webhook events. If empty, signature validation is skipped and a
	// warning is logged - acceptable for local development only. Startup
	// validation must reject this configuration outside development.
	WebhookSecret string `mapstructure:"webhookSecret"`
}

// PaymentConfig holds configuration for payment provider webhook integrations.
//
// Both secrets are required in non-development environments. The middleware
// layer (middleware.RecurrenteWebhookAuth, middleware.PayPalWebhookAuth)
// enforces the respective verification algorithms at request time.
type PaymentConfig struct {
	// RecurrenteWebhookSecret is the HMAC-SHA256 signing secret configured in
	// the Recurrente webhook dashboard. Requests are verified against the
	// X-Recurrente-Hmac-Sha256 header. Set via WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET.
	RecurrenteWebhookSecret string `mapstructure:"recurrenteWebhookSecret"`
	// PayPalWebhookID is the webhook ID from the PayPal developer dashboard.
	// It is embedded in the signed message during RSA certificate verification.
	// Set via WCQ_PAYMENT_PAYPALWEBHOOKID.
	PayPalWebhookID string `mapstructure:"paypalWebhookID"`
}

// TracingConfig controls distributed tracing via OpenTelemetry.
//
// When Enabled is false (the default) a no-op TracerProvider is registered:
// all OTel API calls compile and run correctly but produce no spans and make
// no network connections. This is appropriate for local development.
//
// For production, set WCQ_TRACING_ENABLED=true and point
// WCQ_TRACING_OTLPENDPOINT at your OTel Collector or backend (Grafana Tempo,
// Jaeger, DataDog Agent). The endpoint must expose the OTLP HTTP receiver on
// port 4318 by default.
type TracingConfig struct {
	// Enabled controls whether spans are collected and exported.
	// Default: false. Set WCQ_TRACING_ENABLED=true in production.
	Enabled bool `mapstructure:"enabled"`
	// OTLPEndpoint is the base URL of the OTLP HTTP receiver.
	// Example: "http://tempo:4318". Required when Enabled is true.
	OTLPEndpoint string `mapstructure:"otlpEndpoint"`
	// ServiceName is stamped on every span. Defaults to "world-cup-quiniela".
	ServiceName string `mapstructure:"serviceName"`
	// ServiceVersion is stamped on every span. Defaults to "1.0.0".
	ServiceVersion string `mapstructure:"serviceVersion"`
	// SampleRate is the fraction of traces to record (0.0–1.0).
	// Default: 1.0 (record every trace). Reduce for high-traffic production.
	SampleRate float64 `mapstructure:"sampleRate"`
}

// MetricsConfig controls Prometheus metrics collection via the OTel SDK.
//
// When Enabled is false (the default) a noop MeterProvider is registered:
// all OTel Meter calls compile and run without side-effects, and no Prometheus
// registry is created. This is appropriate for local development.
//
// For production, set WCQ_METRICS_ENABLED=true. The /metrics endpoint will
// be registered and Prometheus can scrape it.
type MetricsConfig struct {
	// Enabled controls whether the Prometheus exporter is active.
	// Default: false. Set WCQ_METRICS_ENABLED=true in production.
	Enabled bool `mapstructure:"enabled"`
	// Namespace is the metric name prefix applied to all instruments.
	// Example: "wcq" → "wcq_notification_sse_connections".
	// Defaults to "wcq" when empty.
	Namespace string `mapstructure:"namespace"`
}
