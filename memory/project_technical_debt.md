---
name: project-technical-debt
description: Resolution status for DT-15 through DT-20 technical debt items on branch refactor/resolve-technical-debt
metadata:
  type: project
---

## DT-15/16/17 (resolved — migration 000105)
PushDigestGate (m000105), SSE hub OTel+admin stats, global snapshot semaphore.

## DT-18: system_params_history archival (resolved — migration 000106)
- History table + repository + service layer complete
- `BulkSet` now captures old values during validation and records one history row per key
- New param `system.param_history_retention_days` (default 90, seeded in m000106)
- `Purger.PurgeOldParamHistory` added; worker reads param at startup and calls it in `monitorPurge`

**Why:** system_params_history grew without retention; BulkSet silently skipped history.
**How to apply:** Migration 000106 must run before deploying; worker restart picks up new retention param.

## DT-19: Escalation package (resolved)
- Created `internal/notification/escalation/` with `StaleOps` escalator
- `StaleOps` encapsulates bank-transfer + withdrawal stale alerting with injectable clock and zero-threshold short-circuit
- `scheduler/jobs.go::StaleEscalation` now delegates to `escalation.NewStaleOps` — logic is independently tested in `stale_ops_test.go`

**Why:** Logic was embedded in scheduler jobs, untestable in isolation; package existed only in roadmap.

## DT-20: Performance benchmarks (resolved)
- `internal/service/system_param_bench_test.go` — 7 benchmarks covering cache hit/miss, Set, BulkSet, hook dispatch, GetInt, TTL eviction
- `internal/notification/push_digest_bench_test.go` — 6 benchmarks covering all gate paths + PriorityOf lookup table
- `internal/api/bench_test.go` — 4 HTTP handler benchmarks establishing latency baseline (~43 µs/op health, ~28 µs/op routing, ~22 µs/op 404)

**Baselines (i7-8850H, 3 GOMAXPROCS):**
- SystemParamGet cache hit: 53 ns/op (parallel)
- PushDigestGate P0 bypass: 1.4 ns/op
- GET /health full stack: 43 µs/op (parallel)

**Why:** No baseline meant SLOs were aspirational. Benchmarks live in `_bench_test.go` files, run with `go test -bench=.`
