package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// IPAllower is the interface accepted by RateLimitByIP. It is satisfied by
// LimiterStore and RedisRateStore, allowing both in-process and distributed
// IP rate limiting without changing the middleware call site.
type IPAllower interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfterSecs int)
}

// IPRateLimiter holds the two-tier IP rate limiting stores and the OTel
// counter used to track blocked requests per layer. Constructed once at
// startup via NewIPRateLimiter and shared across all requests.
type IPRateLimiter struct {
	global       IPAllower
	webhook      IPAllower
	blockedTotal metric.Int64Counter // wcq_ip_rate_limit_blocked_total{layer=...}
	failOpen     metric.Int64Counter // wcq_ip_rate_limit_fail_open_total
	log          *zap.Logger
}

// NewIPRateLimiter constructs an IPRateLimiter wired to the given stores.
// meter may be nil in tests (OTel counter registration is a no-op on error).
func NewIPRateLimiter(global, webhook IPAllower, meter metric.Meter, log *zap.Logger) *IPRateLimiter {
	l := &IPRateLimiter{global: global, webhook: webhook, log: log}
	if meter != nil {
		l.blockedTotal, _ = meter.Int64Counter(
			"wcq_ip_rate_limit_blocked_total",
			metric.WithDescription("Number of requests blocked by the per-IP rate limiter. "+
				"Label layer=global is the L1 all-routes bucket; layer=webhook is the L2 "+
				"stricter bucket applied only to /webhooks/* routes."),
		)
		l.failOpen, _ = meter.Int64Counter(
			"wcq_ip_rate_limit_fail_open_total",
			metric.WithDescription("Number of IP rate limit checks that failed open "+
				"due to a store error (in-process fallback active)."),
		)
	}
	return l
}

// Global returns middleware that enforces the L1 per-IP rate limit across
// all routes. It must be placed after TrustedClientIP (which sets r.RemoteAddr
// to the authoritative client IP) in the middleware chain.
//
// When the IP cannot be extracted (empty RemoteAddr), the request passes
// through without consuming a token — this is a safe fail-open for edge cases
// such as Unix-socket connections in tests.
func (l *IPRateLimiter) Global() func(http.Handler) http.Handler {
	return l.layer("global")
}

// Webhook returns middleware that enforces the tighter L2 per-IP rate limit
// on webhook routes only. It must be applied as a per-route middleware (via
// r.With) on each /webhooks/* handler, not as a global middleware.
func (l *IPRateLimiter) Webhook() func(http.Handler) http.Handler {
	return l.layer("webhook")
}

func (l *IPRateLimiter) layer(name string) func(http.Handler) http.Handler {
	var store IPAllower
	switch name {
	case "webhook":
		store = l.webhook
	default:
		store = l.global
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if ip == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfter := store.Allow(r.Context(), fmt.Sprintf("ip:%s:%s", name, ip))
			if !allowed {
				if l.blockedTotal != nil {
					l.blockedTotal.Add(r.Context(), 1,
						metric.WithAttributes(attribute.String("layer", name)),
					)
				}
				l.log.Warn("IP rate limit exceeded",
					zap.String("layer", name),
					zap.String("ip", ip),
					zap.String("path", r.URL.Path),
				)
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				WriteError(w, r, l.log, apperrors.RateLimited(apperrors.MsgRateLimited))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
