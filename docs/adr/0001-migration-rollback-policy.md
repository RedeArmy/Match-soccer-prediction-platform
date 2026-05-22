# ADR 0001 — Migration Rollback Policy

**Status:** Accepted  
**Date:** 2026-05-22  
**Deciders:** Engineering team

---

## Context

The project uses sequential, numbered SQL migrations (currently 107 pairs). Every
migration ships a `.up.sql` and a `.down.sql` file. The CI pipeline exercises a full
`up → down → up` round-trip on every pull request targeting `develop` or `main`,
ensuring that each `.down.sql` is syntactically and semantically correct at merge time.

Despite this, operational rollback has a hard practical limit. Once a migration is
deployed to production and data has been written against the new schema, rolling back
further than the most recent few migrations is rarely safe:

- Column drops and table renames are irreversible once dependent writes have occurred.
- Foreign-key and check-constraint changes affect existing rows on rolldown.
- Enum additions cannot be un-added once the new value appears in stored data.
- At 107 migrations, running `migrate down` interactively to, say, migration 095
  under incident pressure is error-prone and extremely slow.

---

## Decision

### 1. Operational rollback window: N−3

The team commits to maintaining **at most three migrations** as a safe rollback
window at any given time. This means:

- Migrations `N`, `N−1`, and `N−2` (the three most recently deployed) may be rolled
  back in production if a critical regression is discovered before data drift makes
  reversal unsafe.
- Rolling back beyond `N−3` requires a **forward-fix migration** (see §3).

The N−3 window reflects the typical deployment cadence: three migrations is enough
to cover a single feature branch, a hotfix, and a concurrent background task.

### 2. Authoring discipline for down migrations

Every `.down.sql` must:

- Be **idempotent** — safe to run twice (use `DROP TABLE IF EXISTS`,
  `DROP COLUMN IF EXISTS`, etc.).
- **Not delete data** unless the up migration added a nullable or default-valued
  column and the column is being removed. If data loss on rolldown is unavoidable,
  add a prominent `-- WARNING: data loss on rolldown` comment and note it in the
  PR description.
- Be tested by the CI round-trip job (`make test-migrate-roundtrip`) before merge.
  A broken `.down.sql` blocks the PR.

### 3. Forward-fix migrations beyond the rollback window

If a production regression cannot be resolved by rolling back within the N−3 window,
the remedy is a **forward-fix migration**:

```
migrations/000NNN_fix_<short_description>.up.sql
migrations/000NNN_fix_<short_description>.down.sql
```

The `fix_` infix in the filename signals that this migration was created reactively
to resolve a production issue. Its `.down.sql` may be a no-op (`-- no-op; forward-fix
migration only`) if reversing the fix itself would be harmful.

Forward-fix migrations follow the same authoring rules as regular migrations and must
pass the CI round-trip job before deployment.

### 4. Baseline consolidation

The `migrations/baseline/schema.sql` file provides a consolidated DDL snapshot for
fast bootstrapping of new environments. It must be regenerated whenever the cumulative
number of migrations since the last baseline update exceeds 25, or at the start of
each major tournament preparation phase.

Regeneration command:
```bash
pg_dump --schema-only --no-owner <production_db> > migrations/baseline/schema.sql
```

After regeneration, all existing migration version numbers must still be marked
applied in the `schema_migrations` table so that `--fresh` bootstrap and sequential
migration remain consistent.

---

## Consequences

**Positive**

- Incident response is unambiguous: beyond N−3, engineers write a forward-fix
  migration rather than attempting a multi-step rollback under pressure.
- The forward-fix naming convention creates a visible audit trail in the migration
  history for production incidents.
- The N−3 window aligns with feature-branch merge patterns, so no real operational
  flexibility is lost.

**Negative / trade-offs**

- Engineers must be aware of the window when planning deployments that span multiple
  migration-bearing pull requests. Deploying three migrations in sequence without
  verification narrows the safe rollback window immediately.
- Forward-fix migrations accumulate over time. They should be documented in the
  post-incident review and, where possible, consolidated into the next baseline.

---

## Reference

- CI round-trip job: `.github/workflows/ci.yml` job `test-migrate-roundtrip`
- Migration tool flags: `cmd/migrate/main.go` (`--fresh`, `--seed`, `--baseline`)
- Baseline DDL: `migrations/baseline/schema.sql`
