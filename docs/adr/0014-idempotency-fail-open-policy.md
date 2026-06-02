# ADR 0014 – Idempotency Store Fail-Open Policy

**Status:** Accepted
**Date:** 2026-06-01
**Deciders:** Engineering team

---

## Context

Payment write endpoints (`POST /withdrawals`, `POST /bank-transfers`,
`POST /payment-intents`) are protected by an idempotency middleware
(`internal/middleware/idempotency.go`). The middleware stores a per-key lock in
a Redis-backed `RedisIdempotencyStore` so that a client retrying a timed-out
request is safely deduplicated across all API replicas.

When Redis is unavailable at startup, `Server.ensureIdempotencyStore()`
(`internal/api/server.go`) falls back to an in-process `MemoryStore`. Each
replica maintains its own independent idempotency map. A payment request retried
by the client after a transient timeout could reach a different replica and be
processed twice.

The question being decided: is this acceptable, and if so, what observability
and runbook coverage is required?

---

## Decision

**Fail-open is the correct policy for this system.**

The alternative — fail-closed, refusing all write traffic when Redis is
unavailable — would produce a complete outage of payment endpoints during
every Redis blip. For a quiniela this is strictly worse than the duplicate-
payment risk:

- **The risk is bounded.** A duplicate execution requires the client to
  successfully retry on a *different* replica within the idempotency TTL
  window. The payment_intents token (256-bit random, unique-per-intent)
  is validated by both Recurrente and PayPal; a replayed charge would be
  rejected by the payment provider even if our middleware misses it.
  For bank-transfer proofs, the `reference` unique constraint on
  `balance_ledger` provides a second idempotency layer at the database level.
- **The quiniela scale is low.** The platform serves a known, bounded user
  base. Unlike a public fintech API, the realistic concurrency during a Redis
  outage is low hundreds of requests — not millions.
- **The outage window is short.** Fly.io Redis typically recovers within
  seconds to minutes. The MemoryStore fallback is a transient degraded state,
  not a permanent configuration.

This is explicitly **not** a financial-institution system. The regulatory
requirements are for a sports prediction pool, not banking.

---

## Observability requirement

The fail-open behaviour must never be silent. Two mechanisms ensure visibility:

1. **`wcq_idempotency_degraded_total`** (counter, `internal/api/server_routes.go`).
   Incremented once at startup when the MemoryStore fallback is active, and on
   every request where the per-request Redis check fails.

2. **`WCQIdempotencyDegraded`** (Prometheus alert, `alerting_rules.yml`).
   Fires immediately (`for: 0m`, `severity: critical`) when
   `increase(wcq_idempotency_degraded_total[5m]) > 0`. The runbook entry
   (`docs/runbook.md#WCQIdempotencyDegraded`) prescribes immediate Redis
   investigation and a 15-minute window for duplicate-activity review.

These two signals together ensure that any degradation event pages on-call
within the first Prometheus scrape interval.

---

## Alternatives considered

**Fail-closed (reject all writes on Redis outage):** Rejected. A Redis
connectivity blip would make withdrawals and deposits completely unavailable
during the outage — a worse user experience than a small duplicate risk.

**In-memory deduplication with distributed lock fallback:** Considered.
A leader-election lock (already in use for the worker) could serialise payment
writes across replicas during Redis outages. Rejected as over-engineered: the
existing database-level constraints (unique `reference` column, unique
`payment_intents.token`) provide a second layer that makes duplicates
extremely unlikely even without middleware deduplication.

**Hard-error on missing Redis (fail-safe):** Considered. Rejected for the same
reason as fail-closed — this would make the entire API unavailable when Redis
is down, not just the idempotency guarantee.

---

## Implementation

- `internal/api/server.go:ensureIdempotencyStore` — fallback logic
- `internal/api/server_routes.go:Routes` — startup counter increment
- `observability/prometheus/rules/alerting_rules.yml:WCQIdempotencyDegraded` — alert
- `docs/runbook.md#WCQIdempotencyDegraded` — response runbook

See also **ADR 0012** and **ADR 0013** for the related per-user and IP rate-limiter
fail-open policies, which follow the same rationale.
