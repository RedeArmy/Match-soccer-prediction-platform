# Capacity Model — World Cup Quiniela

This document records the known capacity constraints, theoretical limits, and
measured baselines for the platform. Update it whenever a relevant configuration
value changes or a load test produces new data.

---

## Runtime Configuration (as of writing)

| Parameter | Value | Location |
|---|---|---|
| Fly.io soft_limit | 200 concurrent requests | `fly.toml` |
| Fly.io hard_limit | 250 concurrent requests | `fly.toml` |
| DB pool max connections | 25 | `WCQ_DATABASE_MAXOPENCONNS` |
| DB pool min idle | 5 | `WCQ_DATABASE_MAXIDLECONNS` |
| Connection max lifetime | 5 min | `WCQ_DATABASE_CONNMAXLIFETIME` |
| Redis pool (go-redis default) | 10 | go-redis default |
| API request body limit | 1 MB (default) | `domain.DefaultAPIBodySizeLimitBytes` |
| Upload body limit | 10 MB (default) | `domain.DefaultPaymentMaxUploadBytes` |
| User rate limit | 10 req/s burst 20 | `system_params` |
| IP rate limit (global) | 100 req/s burst 200 | `system_params` |

---

## Theoretical DB Pool Capacity

The DB pool is the primary throughput bottleneck. With `MaxOpenConns = 25`:

```
capacity = max_conns / avg_connection_hold_time
```

| Operation | Avg hold time | Pool capacity |
|---|---|---|
| Health readiness ping | ~1 ms | 25,000 ops/s |
| Simple SELECT (e.g. GET /matches) | ~3 ms | 8,333 ops/s |
| INSERT + SELECT (e.g. POST /predictions) | ~5 ms | 5,000 ops/s |
| Transactional write (e.g. group join) | ~10 ms | 2,500 ops/s |
| Prize distribution (SELECT FOR UPDATE + batch) | ~50–200 ms | 125–500 ops/s |

**At 250 concurrent requests (hard_limit):**  
25 connections shared across 250 requests means each request may queue for
a connection. This is safe as long as the average queue wait plus query time
stays within the request timeout (30 s `WCQ_SERVER_WRITETIMEOUT`).

**Bottleneck scenario:** a match-end event triggers simultaneous scoring for
all groups. Each group leader triggers `DistributePrizesAtomically` (holds a
row-level lock for 50–200 ms). At 10 concurrent prize distributions, all 25
connections are consumed; remaining requests queue. This is acceptable for a
quiniela because prize distribution is an infrequent, admin-triggered batch
operation — it does not compete with user-facing read traffic at peak time.

---

## Observed Benchmarks

Benchmark results are stored in `.bench/baseline.txt` and updated on every
merge to main via the `bench-baseline` job in `deploy.yml`.

Run locally with:

```sh
make bench-compare   # compare current vs baseline
make bench COUNT=6   # run with 6 iterations for stability
```

Key baselines (approximate, single Fly.io machine):

| Benchmark | ~ns/op | Note |
|---|---|---|
| `BenchmarkSystemParamService_GetInt` | ~200 | cached hot path |
| `BenchmarkRankingService_GetLeaderboard` | ~5,000 | cached; DB hit ~50,000 |
| `BenchmarkScoringService_CalculatePoints` | ~100 | pure CPU, no DB |
| `BenchmarkPushDigestGate_Redis` | ~500 | round-trip to Redis |

---

## Load Testing

Manual load tests against a running local server:

```sh
# Start the server first
make run

# Basic health endpoint at Fly.io soft_limit concurrency
make load-test

# Authenticated endpoint
make load-test \
  LOAD_TEST_PATH=/api/v1/matches \
  LOAD_TEST_N=500 \
  LOAD_TEST_C=50 \
  LOAD_TEST_AUTH="Bearer <token>"

# Prize-distribution stress test (admin endpoint)
make load-test \
  LOAD_TEST_PATH=/api/v1/admin/groups/1/distribute-prizes \
  LOAD_TEST_N=10 \
  LOAD_TEST_C=10 \
  LOAD_TEST_AUTH="Bearer <admin-token>"
```

Install `hey` if not present: `go install github.com/rakyll/hey@latest`

### Target RPS by endpoint class

| Endpoint class | Target RPS | Notes |
|---|---|---|
| Health / static | > 5,000 | No DB; limited by HTTP overhead |
| Read (cached) | > 500 | Cache miss adds ~30 ms |
| Write (single row) | > 200 | Subject to user rate limit (10 req/s) |
| Write (transactional) | > 50 | DB pool is the ceiling |
| Admin batch operations | N/A | Intentionally slow; not on user-facing path |

---

## Connection Pool Validation (Integration Tests)

`TestConcurrentReadinessProbes` (in `internal/api/e2e_concurrency_test.go`)
fires 50 concurrent `/health/ready` requests against a real PostgreSQL
testcontainer. The test pool uses `MaxOpenConns = 5` (a deliberate constraint
lower than production's 25) to exercise queuing behaviour: if pool queuing is
broken, some probes return 500 or time out. Run with:

```sh
make test-integration
```

---

## Scaling Recommendations

| Trigger | Action |
|---|---|
| p99 latency > 1 s | Increase `WCQ_DATABASE_MAXOPENCONNS`; add read replica |
| DB pool wait time > 50 ms | Scale DB connections or add caching |
| > 2 Fly.io machines | Redis rate limiting is already cross-replica; DB pool per-replica |
| > 500 groups | Add DB index on `quinielas.prizes_distributed_at` for scheduler query |
| > 10,000 predictions per match | Consider scoring in worker chunks > 500 (tune `WCQ_SCORING_UPDATE_CHUNK_SIZE`) |
