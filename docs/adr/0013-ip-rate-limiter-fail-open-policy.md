# ADR 0013 – IP Rate Limiter Fail-Open Policy

**Status:** Accepted
**Date:** 2026-06-01
**Deciders:** Engineering team

---

## Context

The IP rate limiter uses Redis as its backing store when Redis is configured
(`WCQ_REDIS_ADDR` is set). If Redis becomes unavailable at runtime, each
`Allow()` call returns `(true, 0)` — fail-open — and increments the
`wcq_rate_limit_fail_open_total` counter.

If Redis is not configured at startup, `buildIPRateStore` falls back to an
in-process `LimiterStore` instead. Both cases share the same behavioural
property: **each API replica enforces its own independent token bucket** rather
than a shared cluster-wide quota. Under a 3-replica deployment, the effective
cluster limit becomes `3 × per-replica burst` for the duration of the outage.

The question being decided: is this acceptable, and if so, what observability
is required?

---

## Decision

**Fail-open is the correct policy for this system.**

The alternative — fail-closed, refusing all traffic when Redis is unavailable —
would be a self-induced denial of service: a transient Redis blip would take
down the entire API. For a sports-prediction quiniela this is strictly worse
than temporarily relaxed rate limiting.

**Acceptable risk at quiniela scale:**

The World Cup Quiniela serves a bounded, known user base (invite-only groups,
not a public API). Peak concurrent users during a match-end event are in the
low hundreds, not thousands. At this scale:

- The absolute burst capacity (e.g. 100 RPS × 3 replicas = 300 RPS) remains
  well within the API server and database capacity.
- Scraping or abuse at 300 RPS for the duration of a Redis outage (~minutes)
  would not cause data loss, incorrect scoring, or financial harm. It might
  generate log noise and marginally inflate DB load.
- The cost of fail-closed (total API unavailability) vastly exceeds the risk
  of temporarily relaxed IP limits.

This is explicitly **not** a financial system. Payments are processed by
Recurrente/PayPal via their own rate limiting. The IP limiter protects
unauthenticated endpoints (health checks, Swagger) and webhook callbacks, not
financial transactions.

---

## Observability requirement

Because the fail-open behaviour is silent without instrumentation, two signals
are wired — one for the *transient* case (Redis configured but temporarily
unreachable) and one for the *steady-state* case (Redis never configured):

1. **`wcq_rate_limit_fail_open_total`** (counter, `internal/middleware/rate_limit_redis.go`)
   Incremented on every request that bypasses Redis due to a connectivity error.
   Shared by both the IP and per-user limiters (not distinguished by label —
   either one being degraded is actionable the same way).
   Alert: `WCQRateLimitDegraded` — `increase(wcq_rate_limit_fail_open_total[5m]) > 0`.

2. **`wcq_ratelimit_store_mode{limiter="ip"|"user", mode="redis"|"local"}`**
   (gauge, `internal/api/server_routes.go`, `recordRateStoreMode`)
   Set to 1 at startup for each limiter when Redis is not configured and the
   in-process store is used instead. Recorded separately per limiter (`ip`,
   `user`) even though both key off the same `WCQ_REDIS_ADDR` today, so the
   signal stays correct if that ever changes.
   Alert: `WCQRateLimitLocalMode` — `max by (limiter) (wcq_ratelimit_store_mode{mode="local"}) == 1`
   for 5m. This is the alert that catches "Redis was simply never configured"
   in a multi-replica deployment — a case `WCQRateLimitDegraded` cannot catch,
   since there is no Redis connectivity error to count.
   Complement: `wcq_ratelimit_store_mode{mode="redis"} == 1` confirms the
   preferred path is active after a Redis recovery + process restart.

Both metrics are registered at startup; a no-op provider silently discards
them when `WCQ_METRICS_ENABLED=false` (development default). Both alerts are
wired in `observability/prometheus/rules/alerting_rules.yml` (group
`wcq.alerts.rate_limit`).

---

## Alternatives considered

**Fail-closed (reject all traffic on Redis error):** Rejected. A Redis outage
would cause a complete API outage, which is strictly worse for users than
temporarily relaxed limits.

**Circuit breaker with cached last-known decision:** Rejected. Caching the last
`Allow` result per IP is complex, stale after a few seconds, and still
fail-open in practice for new IPs. Not worth the complexity at this scale.

**Reduce per-replica burst on Redis absence:** Considered. Automatically
halving the burst when local mode is detected would bound the cluster-wide
effective limit. Rejected because: (a) the API server doesn't know the
replica count at runtime, (b) the risk delta is small at quiniela scale,
(c) it adds complexity for marginal benefit.

---

## Implementation

- `internal/middleware/rate_limit_redis.go` — `wcq_rate_limit_fail_open_total`
- `internal/api/server_routes.go` — `recordRateStoreMode`, `buildIPRateStore`, `buildUserRateStore`
- `observability/prometheus/rules/alerting_rules.yml` — `WCQRateLimitDegraded`, `WCQRateLimitLocalMode`
- `docs/adr/0012-rate-limiter-architecture.md` — per-user limiter architecture
