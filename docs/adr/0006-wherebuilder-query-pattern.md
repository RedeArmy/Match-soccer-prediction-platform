# ADR 0006 — whereBuilder Pattern for Dynamic SQL Filters

**Status:** Accepted  
**Date:** 2026-05-25  
**Deciders:** Engineering team

---

## Context

Many list endpoints accept optional filter parameters (actor ID, action, date range,
resource type, etc.) that must be appended to a base SQL query only when the caller
provides them. The classic approach is to start with `WHERE 1=1` and append
`AND col = $N` clauses, incrementing a placeholder counter manually:

```go
args := []any{}
n := 1
q := "SELECT … FROM audit_logs WHERE 1=1"
if f.ActorID != nil {
    q += fmt.Sprintf(" AND actor_id = $%d", n)
    args = append(args, *f.ActorID)
    n++
}
// … repeated for every filter field
```

This pattern has several problems:

1. **Error-prone counter management.** Forgetting to increment `n`, or incrementing
   it in the wrong branch, produces runtime query errors that are hard to catch in
   unit tests.
2. **Cognitive load.** Each new filter requires updating both the `fmt.Sprintf` and the
   counter, in the right order.
3. **Poor readability.** The logic for "which filters are active" is interleaved with
   SQL string construction.

Alternatives considered:

| Option | Pros | Cons |
|---|---|---|
| `WHERE 1=1` + manual counter | Simple, no deps | Error-prone, verbose |
| sqlc (code generation) | Type-safe, no string SQL | Inflexible for dynamic filters; requires regeneration |
| ORM (GORM, Bun) | High-level API | Heavy; fights pgx's connection model |
| **whereBuilder** | Simple, local, explicit | Must be understood by new contributors |

---

## Decision

Use the **`whereBuilder` helper** defined in `internal/repository/query_helpers.go`
for all repository methods with dynamic filter sets.

`whereBuilder` tracks clauses and arguments internally, exposing two methods:

```go
wb := newWhereBuilder()
wb.add("actor_id = $%d", actorID)   // appended only when called
wb.add("action = $%d", action)
clause, args := wb.build()           // returns "WHERE actor_id = $1 AND action = $2", [actorID, action]
```

The placeholder counter is managed internally; callers never touch `$N` directly.
`build()` returns an empty string (not `WHERE 1=1`) when no clauses have been added,
which is safe to concatenate directly into any query.

All new repository methods with optional filters must use `whereBuilder` rather than
the manual counter approach. Existing methods that still use the manual pattern should
be migrated opportunistically when they are modified for other reasons.

---

## Consequences

**Positive**

- Placeholder counter errors are structurally impossible; the builder manages `$N`
  internally.
- Adding or removing a filter is a single `wb.add(…)` call with no counter bookkeeping.
- `build()` returning an empty string for zero clauses eliminates the `WHERE 1=1` hack.

**Negative / trade-offs**

- New contributors must learn the local convention. The helper is documented in
  `query_helpers.go` and referenced in this ADR.
- `whereBuilder` is not a query builder for `SELECT` or `ORDER BY` clauses — only
  for the `WHERE` predicate. Complex queries with dynamic column sets still require
  manual construction.
- The helper uses `fmt.Sprintf` internally, so the clause template must contain
  exactly one `%d` placeholder per argument. Mismatches produce a runtime
  format-string error rather than a compile-time error.

---

## Reference

- Helper implementation: `internal/repository/query_helpers.go`
- Example usage: `internal/repository/audit_log_repository.go` (`List`, `ListByEntity`)
- Rationale comment: `internal/repository/query_helpers.go` (package doc)
