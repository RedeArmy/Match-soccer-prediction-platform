package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// limiterTTL is the inactivity window after which an entry is eligible for eviction.
const limiterTTL = 10 * time.Minute

// Allower is the abstraction accepted by RateLimitByUserID. It allows
// swapping the in-process token-bucket store for a Redis-backed implementation
// without changing the middleware call site.
type Allower interface {
	// Allow reports whether the request for key is within the rate limit.
	// When false, retryAfterSecs is the number of seconds the caller should
	// wait before retrying — used to populate the Retry-After response header.
	Allow(ctx context.Context, key string) (allowed bool, retryAfterSecs int)
}

// LimiterStore is a concurrent map from arbitrary key to a per-key token-bucket
// rate limiter. Stale entries are evicted lazily every ~2000 Allow calls.
// It is safe for concurrent use by multiple goroutines.
// LimiterStore implements Allower.
type LimiterStore struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	r       rate.Limit
	burst   int
	callN   uint64
}

type limiterEntry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// NewLimiterStore creates a store where each unique key gets its own token
// bucket that refills at ratePerSec tokens per second with a maximum burst of
// burst. ratePerSec is typically sourced from the api.rate_limit_rate_per_sec
// system parameter (domain.ParamKeyAPIRateLimitRatePerSec); burst from
// api.rate_limit_burst (domain.ParamKeyAPIRateLimitBurst).
func NewLimiterStore(ratePerSec float64, burst int) *LimiterStore {
	return &LimiterStore{
		entries: make(map[string]*limiterEntry),
		r:       rate.Limit(ratePerSec),
		burst:   burst,
	}
}

// NewUnlimitedLimiterStore returns a LimiterStore that never throttles any key.
// Intended for tests that exercise the full middleware chain without triggering
// 429 responses, and for internal/admin tooling exempt from rate limiting.
func NewUnlimitedLimiterStore() *LimiterStore {
	return NewLimiterStore(math.MaxFloat64, math.MaxInt)
}

// Allow implements Allower. ctx is unused (token-bucket is synchronous).
// Returns (permitted, retryAfterSeconds). When permitted is false,
// retryAfterSeconds is the ceiling of the delay until the next token is
// available — suitable for the Retry-After response header.
func (s *LimiterStore) Allow(_ context.Context, key string) (bool, int) {
	s.mu.Lock()

	s.callN++
	if s.callN%2000 == 0 {
		s.evictLocked()
	}

	e, ok := s.entries[key]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(s.r, s.burst)}
		s.entries[key] = e
	}
	e.lastSeen = time.Now()

	res := e.lim.Reserve()
	s.mu.Unlock()

	delay := res.Delay()
	if delay > 0 {
		res.Cancel()
		return false, int(math.Ceil(delay.Seconds()))
	}
	return true, 0
}

// evictLocked removes entries not accessed within limiterTTL. Must be called
// with s.mu held.
func (s *LimiterStore) evictLocked() {
	now := time.Now()
	for k, e := range s.entries {
		if now.Sub(e.lastSeen) > limiterTTL {
			delete(s.entries, k)
		}
	}
}

// RateLimitByUserID returns a middleware that enforces a per-user rate limit
// using the Clerk subject ID stored in context by RequireAuth. It must be
// placed after RequireAuth in the middleware chain so the subject is available.
// When no subject is present the request passes through without consuming a
// token — the caller's auth middleware is responsible for rejecting
// unauthenticated requests.
//
// Rejected requests receive HTTP 429 with a Retry-After header indicating
// the number of seconds until the next token is available.
func RateLimitByUserID(store Allower, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, ok := UserIDFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			permitted, retryAfter := store.Allow(r.Context(), subject)
			if !permitted {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				WriteError(w, r, log, apperrors.RateLimited(apperrors.MsgRateLimited))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
