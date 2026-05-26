# ADR 0005 — Redis Leader Election for Multi-Replica Worker

**Status:** Accepted  
**Date:** 2026-05-25  
**Deciders:** Engineering team

---

## Context

The worker process runs scheduled jobs (score publication, snapshot generation,
outbox processing, leaderboard refresh) that must execute exactly once per schedule
interval, even when multiple worker replicas are deployed for high availability.
Without coordination, every replica would run every job simultaneously, causing:

- Duplicate notifications delivered to users.
- Race conditions when updating leaderboard snapshots.
- Database contention from concurrent writes to the same rows.

PostgreSQL advisory locks and table-based leader election were considered. Redis
`SET NX PX` (set-if-not-exists with expiry) was chosen because:

1. It is a single network round-trip — cheaper than a `BEGIN … COMMIT` advisory lock.
2. The key expires automatically even if the worker holding it crashes, preventing
   a stuck lock that blocks all replicas indefinitely.
3. Redis is already a required infrastructure dependency (event bus driver); adding a
   PostgreSQL advisory lock dependency on a separate schema object would introduce a
   new failure surface.
4. The lock is scoped to a logical job name (e.g. `worker:lock:snapshot`), so
   different jobs can be held by different replicas simultaneously — maximising
   throughput.

---

## Decision

Use **Redis `SET NX PX` distributed locking** for all scheduled jobs that must not
run concurrently across replicas.

The `RedisSnapshotLocker` in `cmd/worker/main.go` wraps this pattern:

```
TRYLOCK: SET <key> <workerID> NX PX <ttlMs>  → true if acquired, false if already held
UNLOCK:  DEL <key>
```

The worker ID is a random UUID generated at process start. If `UNLOCK` is called
after the TTL has expired (e.g. because a job ran longer than expected), the DEL is
a safe no-op — it will delete a key that no longer exists or that belongs to the next
holder.

The lock TTL is set conservatively longer than the expected job runtime. If a job
consistently approaches the TTL, the TTL is raised in system params rather than
removing the lock.

When Redis is unavailable, the locker returns an error. The caller (`runSnapshot`)
degrades gracefully: it logs a warning and runs the job locally without the lock.
This means a Redis outage causes duplicate job execution (both replicas run), which
is acceptable for idempotent jobs (snapshots can be recomputed) but is logged at
WARN level so the outage is visible.

---

## Consequences

**Positive**

- Single-replica execution of scheduled jobs without a dedicated leader-election
  service or PostgreSQL schema objects.
- Lock expiry prevents split-brain from lasting longer than the TTL.
- Degradation path: Redis outage causes duplicate execution, not a complete halt,
  for idempotent jobs.

**Negative / trade-offs**

- Non-idempotent jobs (e.g. notification delivery) must not degrade to unlocked
  execution. The outbox processor relies on `SELECT … FOR UPDATE SKIP LOCKED` in
  PostgreSQL instead of the Redis lock for its single-execution guarantee.
- Clock drift between replicas can cause the lock to be held slightly longer or
  shorter than the configured TTL. This is acceptable at the millisecond scale of
  typical drift.
- The unlock step is not atomic with a lock-ownership check. A replica that runs
  longer than the TTL may delete a lock held by a different replica. The worker ID
  in the lock value is not currently validated on DEL — this is a known gap and
  should be addressed by replacing `DEL` with a Lua `GET-then-DEL` script if
  job runtimes approach the TTL in practice.

---

## Reference

- Redis locker implementation: `cmd/worker/main.go` (`RedisSnapshotLocker`)
- Snapshot job coordination: `cmd/worker/main.go` (`runSnapshot`)
- Lock TTL configuration: system param `worker.snapshot_lock_ttl_ms`
- Degradation behaviour: `cmd/worker/main.go` (`TestRunSnapshot_LockError_DegradesToLocalAndRunsSnapshot`)
