# ADR 0004 — Transactional Outbox Pattern for Notification Delivery

**Status:** Accepted  
**Date:** 2026-05-25  
**Deciders:** Engineering team

---

## Context

Sending user notifications (email, web push) is a side effect of business operations
such as score publication, payment validation, and group membership changes. The naive
implementation — call the notification sender inside the database transaction that
commits the business event — creates a dual-write problem:

- If the sender call fails after the transaction commits, the notification is silently
  lost.
- If the sender call is made before the transaction commits and the commit fails, a
  notification is sent for an event that never happened.
- Direct in-transaction HTTP calls increase transaction duration, tying up connection
  pool slots and increasing the probability of deadlocks.

The application runs multiple replicas (API and worker processes) that must not
duplicate notifications when processing the same event.

Alternatives considered:

| Option | Exactly-once delivery | Survives process crash | Multi-replica safe |
|---|---|---|---|
| In-transaction HTTP call | No (dual-write) | No | No |
| Redis pub/sub fanout | No (fire-and-forget) | No | Partial |
| Message queue (SQS, RabbitMQ) | At-least-once with ack | Yes | Yes |
| **Transactional outbox** | **At-least-once with idempotency** | **Yes** | **Yes** |

---

## Decision

Use the **transactional outbox pattern** for all user-facing notifications.

1. **Write phase:** When a business operation commits, it also inserts one or more rows
   into the `notification_outbox` table within the **same database transaction**. If
   the transaction rolls back, the outbox entries are rolled back with it. If it
   commits, the outbox entries are guaranteed to exist.

2. **Delivery phase:** The worker process polls `notification_outbox` for rows with
   `status = 'pending'` (or picks them up via `pg_notify`). It claims a batch by
   updating their status to `'processing'`, delivers the notification, then marks rows
   `'delivered'` on success or `'failed'` on permanent failure. Rows that fail are
   retried up to a configurable limit before being moved to the dead-letter queue
   (`notification_outbox_dlq`).

3. **Idempotency:** Each outbox row has a stable `idempotency_key` derived from the
   event identity (e.g. `score_published:<match_id>:<user_id>`). The delivery layer
   checks this key to avoid re-sending if a row is claimed by two workers during a
   failover (see `pkg/idempotency`).

4. **DLQ monitoring:** Dead-letter entries trigger an alert via the n8n webhook
   (`WCQ_N8N_ALERTWEBHOOKURL`). On-call investigates the root cause and replays
   entries manually after fixing the underlying issue.

The outbox table and polling logic live in `internal/notification/outbox/`.

---

## Consequences

**Positive**

- Notifications are guaranteed to be attempted at least once for every committed
  business event, even if the process crashes immediately after the commit.
- The business transaction does not depend on the notification sender being available;
  the sender can be down for minutes without affecting business logic.
- Multi-replica safe: at-most-once delivery per replica is achieved via row-level
  locking (`SELECT … FOR UPDATE SKIP LOCKED`).

**Negative / trade-offs**

- Delivery is asynchronous: there is a polling delay (or pg_notify latency) between
  the business event and the notification reaching the user. This is acceptable for
  tournament notifications but must be documented for any latency-sensitive use case.
- The outbox table grows indefinitely if delivered rows are not purged. A background
  purge job (`purgeDeliveredOutboxRows`) runs nightly to reclaim space.
- Adding a new notification type requires both a business-layer outbox write and a
  worker-side handler. Forgetting either half creates silent gaps.

---

## Reference

- Outbox table: `migrations/000089_notification_outbox.up.sql`
- Delivery loop: `internal/notification/outbox/processor.go`
- Idempotency package: `pkg/idempotency/`
- DLQ monitoring: `cmd/worker/main.go` (`monitorDLQ`)
- Dead-letter alert: `internal/notification/escalation/`
