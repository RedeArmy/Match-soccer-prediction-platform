# ADR 0002 — Synthetic vs Persisted Notification Events

**Status:** Accepted  
**Date:** 2026-05-22  
**Deciders:** Engineering team

---

## Context

The notification subsystem emits events via two distinct delivery paths:

1. **Persisted events** — written to the `domain_outbox` table inside (or
   immediately after) a domain transaction.  The outbox worker polls, claims,
   dispatches, and marks each row done or failed.  Failed rows are retried up to
   `messaging.max_retries` times before being moved to `notification_dlq`.  This
   path provides at-least-once delivery for emails, push notifications, in-app
   messages, and admin alerts.

2. **Synthetic events** — published directly to a transport (currently Redis
   Pub/Sub) without any database write.  Delivery is best-effort; there is no
   retry, no DLQ, and no persistence.  A synthetic event is lost if the consumer
   is disconnected at the moment of publication or if Redis is unavailable.

The distinction is operational, not just architectural: persisted events drive
financial and compliance workflows (withdrawal approvals, bank transfer
confirmations), while synthetic events drive UI refresh signals where loss is
acceptable because the client can re-poll on its next interaction.

Without a machine-readable registry, a future engineer adding a new event type
may not know which path applies and may accidentally write a synthetic event to
the outbox — or omit a persisted event from the outbox — with silent correctness
consequences.

---

## Decision

### 1. Canonical registry

Every `notification.EventType` constant that uses the synthetic path must be
listed in `notification.SyntheticEvents`:

```go
var SyntheticEvents = map[EventType]struct{}{
    EventLeaderboardUpdated: {},
}
```

All other constants are implicitly persisted.  Adding a new synthetic event
**requires** an entry in this map and a note in this ADR.

### 2. Runtime guard in outbox.Writer

`outbox.Writer.Write`, `WriteInTx`, `WriteBatch`, and `WriteDedup` call
`rejectSynthetic(eventType)` before constructing any SQL.  Passing a synthetic
`EventType` to any of these methods returns an error immediately.  This prevents
the silent-miscategorisation failure mode described in the context section.

### 3. Decision rule for new events

| Criterion | Use persisted | Use synthetic |
|---|---|---|
| Consumer must not miss the event (email, push, admin alert) | ✓ | |
| Event drives a financial or compliance action | ✓ | |
| Duplicate delivery on retry is safe | ✓ | |
| Loss is acceptable (UI refresh, best-effort signal) | | ✓ |
| The event has no meaningful payload for retry | | ✓ |
| SSE / WebSocket is the only consumer | | ✓ |

When in doubt, use the persisted path.  Downgrading a persisted event to
synthetic is a forward migration; upgrading a synthetic event to persisted
requires a new migration and careful dedup design.

### 4. Current synthetic events

| EventType | Transport | Rationale |
|---|---|---|
| `leaderboard.updated` | Redis Pub/Sub → SSE | UI refresh signal; client re-polls on next navigation if missed |

---

## Consequences

**Positive**
- A single grep for `SyntheticEvents` immediately shows every event that bypasses
  the outbox reliability path.
- `outbox.Writer` raises an error at call time rather than silently persisting a
  signal that was never meant to be retried.
- New engineers are guided to this ADR by the error message, which names it
  explicitly.

**Negative / trade-offs**
- Any new synthetic event requires two changes: the `EventType` constant and the
  `SyntheticEvents` entry.  This is intentional friction; the cost of two lines
  is lower than the cost of a misfiled event in the outbox.
- `rejectSynthetic` adds a map lookup on every outbox write.  At typical
  notification volumes this is immeasurable.

---

## References

- `internal/notification/events.go` — `SyntheticEvents` registry
- `internal/notification/outbox/writer.go` — `rejectSynthetic` guard
- ADR 0001 — Migration Rollback Policy
