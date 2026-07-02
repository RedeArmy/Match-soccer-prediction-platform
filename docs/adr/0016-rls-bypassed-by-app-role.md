# ADR 0016 – Postgres RLS Protects the Supabase Vector, Not the App

**Status:** Accepted
**Date:** 2026-07-02
**Deciders:** Engineering team

---

## Context

Migration `000204_enable_rls` enables Row-Level Security on every table in the
`public` schema, with no permissive policy defined for any role — a
default-deny for all non-owner roles, plus explicit `REVOKE ALL` for
Supabase's `anon`/`authenticated` roles where they exist.

The Go backend (API + worker) connects to Postgres as the cluster superuser
(or a role granted `BYPASSRLS`), per the DSN configured in `WCQ_DATABASE_DSN`.
Postgres exempts superusers and `BYPASSRLS` roles from RLS enforcement by
definition — every query the application issues sees every row, exactly as
before the migration. **RLS is not a data-access control from the app's own
point of view.**

This raises the obvious question a reviewer will ask: what does enabling RLS
actually buy us, if the one client that talks to the database all day bypasses
it entirely?

---

## Decision

**Keep RLS enabled. It protects a specific vector the app's own connection
role cannot: direct table access via Supabase's PostgREST API.**

Supabase automatically exposes every table in `public` through a PostgREST
HTTP API under the `anon` and `authenticated` Postgres roles. That exposure
exists independently of anything the Go backend does — it is a property of
the hosting platform, active as soon as a table exists, and reachable by
anyone who has the project's Supabase URL (not a secret; it appears in
client-side config in some Supabase setups, and is discoverable regardless).
Without RLS, `anon`/`authenticated` have Postgres's normal default table
privileges and can read/write directly through that HTTP endpoint — completely
outside the application's auth middleware, KYC gates, balance-ledger
invariants, and audit logging.

RLS (plus the belt-and-suspenders `REVOKE ALL` in the migration) closes that
specific hole: `anon`/`authenticated` get a default-deny, so the PostgREST
vector returns nothing. This is the entire purpose of the migration — it was
never intended to constrain the application's own database role, which has
its own trust boundary (network isolation, IAM/credential access to
`WCQ_DATABASE_DSN`) enforced elsewhere, not by Postgres row policies.

**This is not a gap to be closed by adding RLS policies for the app's own
role.** Writing per-table policies that the app's `BYPASSRLS`/superuser role
would need to satisfy is not how RLS works (bypass roles ignore policies
regardless of whether any are defined), and switching the app to a
non-bypass role scoped by policy would mean re-deriving, in SQL, every
authorization decision the service layer already makes in Go (group
membership, KYC tier, admin role, ownership checks) — a large, high-risk
rewrite for a system that is not exposed to untrusted direct-SQL clients in
the first place. The one client that matters for defense-in-depth here is
PostgREST, which RLS already fully covers.

---

## Consequences

- A Postgres-level SQL injection in the Go backend is **not** mitigated by
  RLS — the app's own role bypasses it. That class of bug must be prevented
  the way it already is: parameterized queries via pgx everywhere (no other
  pattern exists in this codebase; see `internal/repository/query_helpers.go`).
- If the app's DB role is ever downgraded from superuser/`BYPASSRLS` to a
  normal role (e.g. during a Supabase migration or a stricter-least-privilege
  push), every table in this migration will start denying the app too, since
  no permissive policy exists for any role. That would be a hard outage, not
  a silent behavior change — worth flagging explicitly if that role change is
  ever made.
- No ORM/query-builder change was needed to ship this — confirmed in the
  migration's own header comment ("No application code changes are
  required") and unchanged by this ADR.

---

## Alternatives considered

**Write per-table RLS policies scoped to the app's own access patterns:**
Rejected. The app's role bypasses RLS by design (superuser/`BYPASSRLS`), so
policies would have no effect unless the role itself changes — see
Consequences above for why that's a separate, much larger decision.

**Drop RLS since "the app bypasses it anyway":** Rejected. This was the
original question motivating this ADR. RLS's value is entirely in the
PostgREST vector, which is real and unrelated to the app's own connection
role.

**Move authorization into Postgres policies as the single source of truth:**
Rejected as disproportionate. This is a sports-prediction quiniela, not a
system exposing raw SQL access to untrusted clients — the existing
Go-service-layer authorization model (group membership, KYC gate, role
checks) is the actual control surface, well-tested, and does not need a
parallel SQL-level reimplementation.

---

## Implementation

- `migrations/000204_enable_rls.up.sql` / `.down.sql`
- No application code changes (by design — see Decision).
