# ADR 0007 — pgxpool + pgx Instead of database/sql

**Status:** Accepted  
**Date:** 2026-05-25  
**Deciders:** Engineering team

---

## Context

Go's `database/sql` package provides a database-agnostic connection pool and query
interface. Applications that target a single database engine (PostgreSQL in this case)
often use `database/sql` with `lib/pq` or `jackc/pgx` as the driver.

The project targets PostgreSQL exclusively and makes use of several PostgreSQL-specific
features:

- `LISTEN/NOTIFY` for cross-process SSE (pg_notify bridge).
- JSONB columns for audit log metadata and system parameters.
- `SELECT … FOR UPDATE SKIP LOCKED` for outbox single-consumer processing.
- PostgreSQL array types for bulk operations.
- `pg_stat_activity`-based health checks.

Using `database/sql` with these features requires either unsafe type assertions to
reach the underlying `*pgx.Conn`, or wrapper libraries that re-expose them through
`database/sql` conventions — neither of which is idiomatic pgx.

---

## Decision

Use **`pgxpool.Pool`** (from `github.com/jackc/pgx/v5/pgxpool`) directly throughout
the application. `database/sql` is not used.

Key consequences of this choice that are reflected in the codebase:

1. **No `sql.Scanner` / `sql.Valuer` interfaces.** pgx uses its own type system
   (`pgtype.JSONB`, `pgtype.Text`, etc.) and codec registration. Custom types are
   registered via `pgx.ConnConfig.TypeMap`.

2. **Transactions are `pgx.Tx`, not `*sql.Tx`.** Repository methods that need
   transactional semantics accept a `pgxpool.Pool` or `pgx.Tx` via a shared
   `PgxExecutor` interface, allowing unit tests to inject a mock.

3. **Named statements are not used.** pgx supports extended query protocol
   (prepared statements) natively; the pool manages statement caching automatically.

4. **OpenTelemetry tracing** is provided by `otelpgx.NewTracer()` wired at pool
   construction (`internal/infrastructure/database/postgres.go`). This traces every
   query, including slow-query attribution in Tempo.

5. **Batch operations** use `pgx.Batch` / `pgx.SendBatch` directly, which is not
   available through `database/sql`.

`pgxpool.Pool` is constructed once in `postgres.go` and injected into all repository
constructors. Repository methods acquire connections from the pool via `pool.Acquire`
or use `pool.QueryRow` / `pool.Exec` directly (the pool implements `pgxpool.Pool`
which satisfies `pgconn.QueryExecer`).

---

## Consequences

**Positive**

- Direct access to all PostgreSQL-specific features without wrappers or type assertions.
- Native `pgx.Batch` for bulk inserts (scoring batch, push subscription prune).
- First-class `pg_notify` support via `conn.WaitForNotification`.
- `otelpgx` provides accurate per-query trace spans without instrumentation shims.

**Negative / trade-offs**

- The codebase is permanently coupled to PostgreSQL. Switching databases would require
  replacing all repository implementations, not just the driver. This is an accepted
  constraint: the project is designed for PostgreSQL and the tournament lifecycle does
  not require database portability.
- `database/sql`-based third-party libraries (e.g. `golang-migrate`) must be given a
  `*sql.DB` adapter. The migration tool (`cmd/migrate`) opens a separate `*sql.DB`
  connection for this purpose using `stdlib.OpenDB(pgx.ConnConfig)`.
- Mock testing requires either a `pgxmock` library or testcontainer-based integration
  tests. The project uses testcontainers for repository-layer tests; service-layer
  tests use hand-written stubs.

---

## Reference

- Pool construction: `internal/infrastructure/database/postgres.go`
- OTel tracer wiring: `internal/infrastructure/database/postgres.go:100`
- PgxExecutor interface: `internal/repository/executor.go`
- Batch insert example: `internal/repository/prediction_score_log_repository.go` (`InsertScoringBatch`)
- Migration tool SQL DB adapter: `cmd/migrate/main.go`
