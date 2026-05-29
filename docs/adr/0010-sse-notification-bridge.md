# ADR 0010 – SSE Notification Bridge Architecture

**Status:** Accepted
**Date:** 2026-05-28
**Deciders:** Engineering team

---

## Context

The API server maintains an in-process SSE hub (`internal/notification/hub/Hub`)
that delivers real-time notifications to connected browser clients. Notifications
originate from database writes in the worker process or from admin actions in the
API process itself. A delivery mechanism is needed to bridge the two.

Two mechanisms were evaluated:

1. **PostgreSQL `pg_notify` bridge** — a long-lived LISTEN connection per API
   replica that receives `NOTIFY user_notifications` payloads from the database
   trigger and fans them out to the local SSE hub.
2. **Redis Pub/Sub bridge** — each replica subscribes to the `user_notifications`
   Redis channel; any process (worker or API) publishes to the channel and every
   replica fans out to its local SSE connections.

---

## Decision

Both bridges are implemented and the selection is automatic at startup
(`cmd/api/main.go:startNotifyBridge`):

- **When `WCQ_REDIS_ADDR` is set** → `runRedisBridge` is used. Redis reconnects
  transparently in < 100 ms; no long-lived database connection is held. Every
  replica receives every published message and delivers to locally-connected users.
  This is the production path for all Fly.io deployments where Redis is required.

- **When Redis is not configured** → `runPgNotifyBridge` is used. This works
  correctly for single-replica deployments. The bridge reconnects with exponential
  backoff (1 s → 30 s) on connection loss.

---

## Multi-replica constraint

SSE delivery is **only reliable across multiple replicas when Redis is
configured**. If Redis is absent and the platform runs more than one replica,
a user's SSE connection and the pg_notify signal may land on different replicas,
silently dropping the notification. The pg_notify path logs a warning at startup
when a non-development environment is detected without Redis.

Fly.io deployments always configure Redis (`WCQ_REDIS_ADDR` is a required secret
in `fly.toml`), so this constraint does not apply to production.

---

## Alternatives considered

**Sticky sessions (Fly.io `session_affinity`):** Routes all requests from a
given user to the same replica. Rejected because it adds routing state, breaks
rolling deployments, and does not prevent delivery gaps during machine restarts.

**Database polling per connection:** Each SSE handler polls `domain_outbox` for
new entries. Rejected because it creates O(connections) polling queries against
the database — untenable at > 100 concurrent SSE clients.

**Dedicated SSE service:** A single stateful process owning all SSE connections.
Rejected as premature — the Redis fan-out pattern achieves the same property
without a new deployment unit.

---

## Consequences

- Production requires Redis (`WCQ_REDIS_ADDR` must be set). This is already
  enforced by the Fly.io secret requirements in `fly.toml`.
- Development without Redis uses pg_notify (single-replica, acceptable).
- Both bridges require panic recovery in their goroutines to avoid process
  termination on malformed payloads (see ADR 0010 implementation note and
  the corresponding fix in `internal/api/server_bridge.go`).

---

## Implementation

- `internal/api/server_bridge.go` — `runPgNotifyBridge`, `runRedisBridge`
- `internal/api/server.go` — `StartPgNotifyBridge`, `StartRedisBridge`,
  `StopPgNotifyBridge`
- `cmd/api/main.go:startNotifyBridge` — bridge selection logic
