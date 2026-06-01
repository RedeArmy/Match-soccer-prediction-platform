# ADR 0012 – Per-User Rate Limiter Architecture

**Status:** Accepted
**Date:** 2026-05-28
**Deciders:** Engineering team

---

## Context

The API requires per-user rate limiting to prevent individual accounts from
exhausting backend capacity. The limiter must:
- Identify requests by Clerk subject (user ID from the JWT).
- Enforce limits across all API replicas when running horizontally.
- Not block all users when the rate-limiting infrastructure is unavailable.

---

## Decision

Two interchangeable implementations are provided, selected at startup:

**`LimiterStore`** (`internal/middleware/rate_limit.go`):
An in-process token-bucket store using `golang.org/x/time/rate`. Each unique
key gets its own limiter. Stale entries are evicted lazily every 2,000 `Allow`
calls. Limits are **not shared across replicas** — suitable for development or
single-replica deployments.

**`RedisRateStore`** (`internal/middleware/rate_limit_redis.go`):
A fixed-window counter backed by Redis (`INCR` + `EXPIRE` per 1-second window).
Limits are **enforced cluster-wide**. Redis errors are fail-open: a connectivity
problem returns `(true, 0)` so a Redis outage does not block all users. This is
the correct choice because the alternative (fail-closed) would be a self-induced
outage.

Selection in `internal/api/server_routes.go:Routes()`:
- If `rc != nil` (Redis configured) → `RedisRateStore`
- Otherwise → `LimiterStore` with a startup warning

Rate limit parameters (`ratePerSec`, `burst`) are read from system params at
startup and require a process restart to change (`is_runtime=FALSE`).

---

## Alternatives considered

**Single shared Redis-only limiter:** Rejected because it makes Redis a hard
dependency. A Redis outage would prevent all authenticated API access.

**nginx/load-balancer-level rate limiting:** Rejected at this stage — the
Clerk subject is only available after JWT validation, which happens inside the
application. Delegating to the load balancer would require forwarding the JWT
for validation there.

**Leaky bucket instead of token bucket:** No meaningful difference at the
burst sizes we operate. Token bucket is simpler and already available via
`golang.org/x/time/rate`.

---

## Known limitations

None outstanding. The original `INCR` + `EXPIRE` non-atomicity (risk: unbounded
key growth on crash) was resolved by replacing the two-command sequence with the
`incrWindowScript` Lua script in `rate_limit_redis.go`. The TTL is now set
atomically on the first increment, matching the pattern used in
`push_digest_redis.go` and `pkg/dsem/redis.go`.

---

## Implementation

- `internal/middleware/rate_limit.go` — `LimiterStore`, `Allower` interface
- `internal/middleware/rate_limit_redis.go` — `RedisRateStore`
- `internal/api/server_routes.go:Routes()` — selection logic (lines ~230-246)

See also **ADR 0013** for the IP-specific fail-open policy and the observability
signals (`wcq_rate_limit_fail_open_total`, `wcq_ip_ratelimit_store_mode`) that
make the fallback visible in Grafana.
