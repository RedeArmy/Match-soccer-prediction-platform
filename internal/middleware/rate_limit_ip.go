package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// IPRateLimiter implements a fixed-window rate limiter keyed by client IP
// address. It is designed to operate at two layers:
//
//   - L1 (global): applied to every request before route dispatch, capping
//     requests per IP across the entire API surface.
//   - L2 (webhook): applied to the webhook route group only, enforcing a
//     tighter limit to protect CPU-expensive RSA signature verification.
//
// Cross-replica consistency is achieved via Redis (same INCR+EXPIRE pattern
// as RedisRateStore). When Redis is unavailable or not configured, Allow falls
// back to an in-process token-bucket limiter and the fail-open counter is
// incremented so degraded state is visible in dashboards.
//
// Clock injection (clock.Nower) makes window-boundary behaviour deterministic
// in tests without time.Sleep.
type IPRateLimiter struct {
	rc        redis.UniversalClient // nil when Redis is not configured
	fallback  *LimiterStore         // in-process fallback on Redis errors
	clk       clock.Nower
	log       *zap.Logger
	keyPrefix string // "ip_gl" for L1, "ip_wh" for L2
	limit     int    // max requests allowed per window
	windowSec int64  // window size in seconds

	// OTel counters — nil until RegisterMetrics is called.
	blockedTotal  metric.Int64Counter
	failOpenTotal metric.Int64Counter
}

// NewIPRateLimiter constructs an IPRateLimiter. rc may be nil; in that case
// all Allow calls use the in-process token-bucket fallback. clk must not be
// nil; pass clock.Real{} in production and clock.Frozen{} in tests.
//
// keyPrefix distinguishes the Redis key namespace for each limiter instance
// ("ip_gl" for L1 global, "ip_wh" for L2 webhook). limit is the maximum
// number of requests allowed within windowSec seconds.
func NewIPRateLimiter(
	rc redis.UniversalClient,
	clk clock.Nower,
	log *zap.Logger,
	keyPrefix string,
	limit int,
	windowSec int64,
) *IPRateLimiter {
	return &IPRateLimiter{
		rc:        rc,
		fallback:  newIPFallbackStore(limit, windowSec),
		clk:       clk,
		log:       log,
		keyPrefix: keyPrefix,
		limit:     limit,
		windowSec: windowSec,
	}
}

// newIPFallbackStore creates a LimiterStore whose rate and burst approximate
// the behaviour of the fixed-window IP limiter in single-replica deployments.
func newIPFallbackStore(limit int, windowSec int64) *LimiterStore {
	ratePerSec := float64(limit) / float64(windowSec)
	return NewLimiterStore(ratePerSec, limit)
}

// Configure updates the rate limit parameters. Intended to be called once
// during startup after system_params are loaded, before the first request is
// served. Not safe for concurrent use — call only during Routes() construction.
func (l *IPRateLimiter) Configure(limit int, windowSec int64) {
	l.limit = limit
	l.windowSec = windowSec
	l.fallback = newIPFallbackStore(limit, windowSec)
}

// RegisterMetrics wires OTel Int64Counters for blocked and fail-open events.
// Call once at startup after the global meter provider is initialised, using
// the same meter obtained from otel.GetMeterProvider().Meter("wcq").
//
// Skipping this call is safe: nil counters are no-ops in Allow and Middleware.
func (l *IPRateLimiter) RegisterMetrics(meter metric.Meter) error {
	blocked, err := meter.Int64Counter(
		"wcq_ip_rate_limit_blocked_total",
		metric.WithDescription(
			"Total requests blocked by the IP-based rate limiter. "+
				"Broken down by 'layer' attribute (global, webhook). "+
				"A sustained spike indicates a volumetric attack or webhook replay flood.",
		),
	)
	if err != nil {
		return fmt.Errorf("register wcq_ip_rate_limit_blocked_total: %w", err)
	}

	failOpen, err := meter.Int64Counter(
		"wcq_ip_rate_limit_fail_open_total",
		metric.WithDescription(
			"Requests that bypassed IP rate limiting because Redis was unavailable. "+
				"Non-zero values indicate degraded cross-replica IP limiting. "+
				"Each replica falls back to an in-process token bucket independently.",
		),
	)
	if err != nil {
		return fmt.Errorf("register wcq_ip_rate_limit_fail_open_total: %w", err)
	}

	l.blockedTotal = blocked
	l.failOpenTotal = failOpen
	return nil
}

// Allow reports whether the next request from ip is within the rate limit.
// Returns (true, 0) when allowed. Returns (false, retryAfterSecs) when the
// window is exhausted. retryAfterSecs is the number of seconds until the
// current window closes.
//
// Fail-open behaviour: when Redis returns an error the request is permitted
// and failOpenTotal is incremented. The in-process fallback then applies for
// this call only; subsequent calls re-attempt Redis.
func (l *IPRateLimiter) Allow(ctx context.Context, ip string) (allowed bool, retryAfterSecs int) {
	if l.rc == nil {
		return l.fallback.Allow(ctx, ip)
	}

	now := l.clk.Now()
	bucket := now.Unix() / l.windowSec
	key := fmt.Sprintf("rl:%s:%s:%d", l.keyPrefix, ip, bucket)

	count, err := l.rc.Incr(ctx, key).Result()
	if err != nil {
		if l.failOpenTotal != nil {
			l.failOpenTotal.Add(ctx, 1)
		}
		l.log.Warn("ip rate limiter: Redis INCR failed — failing open",
			zap.String("key_prefix", l.keyPrefix),
			zap.String("ip_prefix", safeIPPrefix(ip)),
			zap.Error(err),
		)
		return true, 0
	}

	if count == 1 {
		// Set TTL to 2× window to guarantee cleanup while tolerating a short
		// race between INCR and EXPIRE on a cold key (same pattern as RedisRateStore).
		ttl := time.Duration(l.windowSec*2) * time.Second
		if err := l.rc.Expire(ctx, key, ttl).Err(); err != nil {
			l.log.Warn("ip rate limiter: EXPIRE failed",
				zap.String("key", key), zap.Error(err))
		}
	}

	if int(count) > l.limit {
		windowEnd := (bucket + 1) * l.windowSec
		retry := int(windowEnd - now.Unix())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	return true, 0
}

// Middleware returns an HTTP middleware function that reads the client IP from
// context (set by StoreClientIP) and enforces this limiter's rate limit.
//
// layer is recorded on the wcq_ip_rate_limit_blocked_total OTel counter and
// in log lines so L1 ("global") and L2 ("webhook") events are distinguishable
// in dashboards and alerts.
//
// Allowed requests have X-RateLimit-Limit set in the response. Blocked
// requests receive HTTP 429 with Retry-After and X-RateLimit-Limit headers.
// When no IP is present in context the request passes through without
// consuming a token, and a debug log line is emitted.
func (l *IPRateLimiter) Middleware(layer string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := repository.ClientIPFromContext(r.Context())
			if ip == "" {
				l.log.Debug("ip_rate_limit: no IP in context, allowing through",
					zap.String("layer", layer),
					zap.String("path", r.URL.Path),
				)
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfterSecs := l.Allow(r.Context(), ip)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(l.limit))

			if !allowed {
				if l.blockedTotal != nil {
					l.blockedTotal.Add(r.Context(), 1,
						metric.WithAttributes(attribute.String("layer", layer)))
				}
				l.log.Warn("ip rate limit exceeded",
					zap.String("layer", layer),
					zap.String("ip_prefix", safeIPPrefix(ip)),
					zap.String("path", r.URL.Path),
					zap.String("method", r.Method),
				)
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSecs))
				WriteError(w, r, l.log, apperrors.RateLimited(apperrors.MsgRateLimited))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// safeIPPrefix returns a privacy-preserving representation of ip for logging.
// For IPv4 addresses only the first two octets are retained (e.g. "203.0.x.x").
// IPv6 addresses and anything that does not parse as IPv4 are fully redacted.
func safeIPPrefix(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + ".x.x"
	}
	return "[ipv6-redacted]"
}
