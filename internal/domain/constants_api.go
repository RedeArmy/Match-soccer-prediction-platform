package domain

// Default values for API, authentication, and circuit-breaker system parameters.
const (
	// DefaultAuthValidationTimeoutSeconds is the JWKS warm-up timeout in seconds.
	DefaultAuthValidationTimeoutSeconds = 5 // auth.validation_timeout_seconds

	// API request limits.
	DefaultAPIBodySizeLimitBytes = 65536 // api.body_size_limit_bytes (64 KB)

	// API rate limiting: per-user token bucket applied at the /api/v1 subrouter.
	// 10 tokens/second with a burst of 30 allows short activity spikes (e.g.
	// loading a dashboard that issues several parallel requests) while preventing
	// sustained high-frequency polling. Both values are read once at process
	// startup (is_runtime=FALSE); a restart is required to change them.
	DefaultAPIRateLimitRatePerSec = 10 // api.rate_limit_rate_per_sec (tokens/second)
	DefaultAPIRateLimitBurst      = 30 // api.rate_limit_burst (max burst size)

	// Idempotency middleware: applied to payment write endpoints.
	// TTL of 24 h gives clients a generous window for safe retry; key length
	// of 255 bytes fits a UUID, hash, or arbitrary client-generated string.
	DefaultAPIIdempotencyTTLHours  = 24  // api.idempotency_ttl_hours
	DefaultAPIIdempotencyKeyMaxLen = 255 // api.idempotency_key_max_len

	// Circuit breaker: PayPal certificate fetcher.
	// Opens after 3 consecutive cert-download failures; stays open for 60 s.
	// PayPal will retry webhook delivery while the circuit is open.
	DefaultBreakerPaypalCertMaxFails    = 3  // breaker.paypal_cert_max_fails
	DefaultBreakerPaypalCertCooldownSec = 60 // breaker.paypal_cert_cooldown_sec

	// Circuit breaker: file store (S3/GDrive/OneDrive).
	// Opens after 5 consecutive storage errors; stays open for 30 s.
	// Handlers return 500 immediately rather than waiting for a network timeout.
	DefaultBreakerFileStoreMaxFails    = 5  // breaker.file_store_max_fails
	DefaultBreakerFileStoreCooldownSec = 30 // breaker.file_store_cooldown_sec

	// Circuit breaker: Redis cache.
	// Opens after 5 consecutive cache errors; stays open for 30 s.
	// While open, service calls bypass the cache and hit the database directly,
	// preventing a Redis outage from returning errors to end users.
	DefaultBreakerCacheMaxFails    = 5  // breaker.cache_max_fails
	DefaultBreakerCacheCooldownSec = 30 // breaker.cache_cooldown_sec
)

// API, authentication, and circuit-breaker system parameter keys.
const (
	// ParamKeyAuthValidationTimeout is the JWKS warm-up timeout in seconds.
	ParamKeyAuthValidationTimeout = "auth.validation_timeout_seconds"

	// ParamKeyAPIBodySizeLimitBytes is the maximum request body size in bytes.
	// Requests exceeding this limit are rejected with 413 to prevent DoS.
	// is_runtime=FALSE: process restart required.
	ParamKeyAPIBodySizeLimitBytes = "api.body_size_limit_bytes"

	// ParamKeyAPIRateLimitRatePerSec is the token-bucket refill rate in tokens per
	// second applied to each authenticated user on the /api/v1 subrouter.
	// is_runtime=FALSE: the LimiterStore is constructed once at startup; a process
	// restart is required to apply a new rate.
	ParamKeyAPIRateLimitRatePerSec = "api.rate_limit_rate_per_sec"

	// ParamKeyAPIRateLimitBurst is the maximum burst size of the per-user token
	// bucket. is_runtime=FALSE: restart required.
	ParamKeyAPIRateLimitBurst = "api.rate_limit_burst"

	// ParamKeyAPIIdempotencyTTLHours is the number of hours a committed
	// idempotency entry is retained in the store.
	// is_runtime=FALSE: the TTL is passed to the store at server startup.
	ParamKeyAPIIdempotencyTTLHours = "api.idempotency_ttl_hours"

	// ParamKeyAPIIdempotencyKeyMaxLen is the maximum byte length of a client-
	// supplied Idempotency-Key header value. is_runtime=FALSE: restart required.
	ParamKeyAPIIdempotencyKeyMaxLen = "api.idempotency_key_max_len"

	// Circuit breaker: PayPal certificate fetcher (is_runtime=FALSE: restart required).
	// ParamKeyBreakerPaypalCertMaxFails is the number of consecutive cert-fetch
	// failures before the circuit opens.
	ParamKeyBreakerPaypalCertMaxFails = "breaker.paypal_cert_max_fails"
	// ParamKeyBreakerPaypalCertCooldownSec is the seconds the circuit stays open
	// before allowing a single trial request.
	ParamKeyBreakerPaypalCertCooldownSec = "breaker.paypal_cert_cooldown_sec"

	// Circuit breaker: file store (S3/GDrive/OneDrive) (is_runtime=FALSE: restart required).
	// ParamKeyBreakerFileStoreMaxFails is the number of consecutive storage errors
	// before the file-store circuit opens.
	ParamKeyBreakerFileStoreMaxFails = "breaker.file_store_max_fails"
	// ParamKeyBreakerFileStoreCooldownSec is the seconds the file-store circuit
	// stays open before allowing a single trial request.
	ParamKeyBreakerFileStoreCooldownSec = "breaker.file_store_cooldown_sec"

	// Circuit breaker: Redis cache (is_runtime=FALSE: restart required).
	// ParamKeyBreakerCacheMaxFails is the number of consecutive cache errors before
	// the cache circuit opens. While open, all cache operations are bypassed so
	// services continue to work against the database.
	ParamKeyBreakerCacheMaxFails = "breaker.cache_max_fails"
	// ParamKeyBreakerCacheCooldownSec is the seconds the cache circuit stays open
	// before allowing a single trial request.
	ParamKeyBreakerCacheCooldownSec = "breaker.cache_cooldown_sec"
)
