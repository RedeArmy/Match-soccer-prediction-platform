# ADR 0008 – balance_ledger partitioning plan

**Status:** Active (monitoring live; migration deferred until ~50M rows)  
**Date:** 2026-05-26  
**Deciders:** Engineering team

---

## Context

`balance_ledger` is an append-only table that grows with every deposit, withdrawal,
prize credit, and reserve/commit pair.  AML velocity checks (migrations 000116–000121,
`SumTransactionsByUserAndPeriod`) scan the table with a 24-hour rolling window filtered
to four credit kinds.  The partial index added in migration 000127 covers this access
pattern today, but once the table exceeds ~50 million rows the index will still require
many leaf-page reads and autovacuum pressure will grow.

---

## Decision

### Immediate (migration 000127 – shipped)

Add a partial B-tree index on `(user_id, created_at DESC)` filtered to the four
velocity-check kinds (`bank_transfer`, `webhook_recurrente`, `webhook_paypal`, `prize`).
This enables Postgres index-only scans for the common 24-hour window query without
touching the ~90 % of rows that are debit/reserve/commit kinds.

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_balance_ledger_velocity
    ON balance_ledger (user_id, created_at DESC)
    WHERE kind IN ('bank_transfer','webhook_recurrente','webhook_paypal','prize');
```

### Future (trigger: ~50 M rows or index bloat > 30 %)

Convert `balance_ledger` to a **declarative range-partitioned** table partitioned by
`created_at`, using monthly or quarterly boundaries.

**Cut-over steps (zero-downtime):**

1. Create `balance_ledger_new` as the partitioned table with identical columns and
   constraints.
2. Attach monthly partitions covering historical data plus one future partition.
3. Migrate rows in batches (e.g. `INSERT INTO balance_ledger_new SELECT … WHERE
   created_at >= 'YYYY-MM-01' AND created_at < 'YYYY-MM-01'::date + interval '1 month'`),
   verifying row counts per partition.
4. Create local versions of all indexes (including `idx_balance_ledger_velocity`) on
   each partition.
5. In a maintenance window: rename `balance_ledger` → `balance_ledger_old`, rename
   `balance_ledger_new` → `balance_ledger`.
6. Keep `balance_ledger_old` for 30 days, then drop.

The partial index on each partition will have the same predicate as today's global
index, so all existing query shapes (`SumTransactionsByUserAndPeriod`,
`ExceedsCumulativeAMLThreshold`, `ListByUser`) continue to work without code changes.

**Partition retention:** Partitions older than the regulatory retention period
(Guatemala UAF requires 5 years) are dropped via a scheduled maintenance job rather
than deleted row-by-row.

---

## Consequences

- **Positive:** Velocity-check queries stay O(index scan) even as the table grows.
  Partition pruning eliminates old-data pages from autovacuum cycles.  Old partitions
  can be archived to cheaper storage.
- **Negative:** `INSERT` performance is marginally slower (partition routing).
  DDL changes (adding a column) must be applied to each partition.  The cut-over
  requires a brief maintenance window.
- **Deferred risk:** The migration is forward-only (ADR 0001); the old table is
  preserved as `balance_ledger_old` until row counts are verified.
