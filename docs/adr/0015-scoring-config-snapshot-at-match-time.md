# ADR 0015 – Scoring Configuration Snapshot at Match Time

**Status:** Accepted
**Date:** 2026-06-01
**Deciders:** Engineering team

---

## Context

The scoring engine resolves point values (exact score, correct outcome, goal
difference, extra-time bonus, penalties bonus) from two sources in priority order:

1. `scoring_rules` table — phase-specific overrides (group stage, round of 16,
   quarter-finals, semi-finals, final).
2. `system_params` — global flat defaults, used when no active phase rule exists.

Once a match is scored for the first time, the exact configuration used is
written to `prediction_score_log` (migration 000109) via `InsertScoringBatch`.
On all subsequent runs for the same match (DLQ replays, manual admin rescores),
`resolveConfig` in `internal/service/scoring_service.go` reads back the earliest
log entry and uses those point values — ignoring any rule changes that occurred
after the original run.

This means **retroactive rule changes do not automatically propagate to already-
scored matches**.

---

## Decision

**Accept the snapshot-at-match-time constraint as the correct design.**

The `prediction_score_log` snapshot exists to guarantee replay idempotency: a
DLQ retry or manual admin rescore must produce the same point values as the
original run to avoid retroactive leaderboard churn. Silently picking up a new
rule during a replay would make scoring non-deterministic and break the scoring
audit trail.

The accepted consequence is that changing scoring rules after matches have been
scored requires a deliberate data migration followed by a manual rescore
operation. This is intentional: retroactive scoring changes are a significant
operational decision that should require explicit human action, not happen as a
side-effect of a config edit.

---

## Trigger for retroactive changes

When a scoring rule must be applied retroactively to already-scored matches:

1. Author a forward-fix migration that updates `prediction_score_log` rows for
   the affected `match_id` values with the corrected point values.
2. Trigger a rescore for affected matches via the admin endpoint or by re-queuing
   match-scored events. `resolveConfig` will now read the updated log entries.
3. Trigger a leaderboard recompute for affected groups (the existing
   `LeaderboardSnapshotService` covers this path).

This flow is intentionally explicit to prevent accidental score revisions.

---

## Alternatives considered

**Always resolve from current rules (no snapshot):** Rejected. DLQ replays and
admin rescores would produce different totals than the original run when rules
change in between. This makes it impossible to answer "what rules were in effect
when this match was originally scored."

**Snapshot on replay only, not initial scoring:** Considered. Rejected because it
requires the log to be written on the first run anyway, and adds a conditional
branch that reduces code clarity without simplifying the invariant.

---

## Implementation

- `internal/service/scoring_service.go:resolveConfig` — snapshot read on re-runs
- `internal/service/scoring_service.go:ScoreMatch` — `InsertScoringBatch` writes snapshot on first scoring
- `internal/repository/prediction_repository.go:GetScoringCfgSnapshot` — log read
- Migration `000109` — `prediction_score_log` table definition

See also **ADR 0002** (synthetic vs. persisted events) for the broader event
replay philosophy this decision is consistent with.
