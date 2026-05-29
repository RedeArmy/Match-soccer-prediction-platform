# ADR 0011 – Prize Distribution Atomicity and TOCTOU Prevention

**Status:** Accepted
**Date:** 2026-05-28
**Deciders:** Engineering team

---

## Context

Prize distribution must be:
1. **Idempotent** — a retry or concurrent call must not double-credit winners.
2. **Correct on stale reads** — the service layer reads `entry_fee` and the
   leaderboard before opening the transaction. An admin settings change
   between that read and the distribution commit could produce prizes based on
   stale data.

The `DistributePrizes` flow in `internal/service/admin_group_service.go` reads
`entry_fee` via `GetByID` and winner data via `GetLeaderboard` before calling
`DistributePrizesAtomically`. This creates a TOCTOU window.

---

## Decision

`DistributePrizesAtomically` (`internal/repository/quiniela_repository.go`) uses
a two-statement approach inside a single PostgreSQL transaction:

1. **`SELECT entry_fee ... WHERE prizes_distributed_at IS NULL FOR UPDATE`** —
   locks the quiniela row. If `prizes_distributed_at IS NOT NULL`, the row is
   not returned → `apperrors.Conflict` (idempotency guard). If the locked
   `entry_fee` differs from `expectedEntryFee` passed by the caller →
   `apperrors.Conflict` (TOCTOU guard).

2. **`UPDATE quinielas SET prizes_distributed_at = NOW()`** — claims the row
   (no IS NULL check needed; the FOR UPDATE lock prevents concurrent claims).

The `expectedEntryFee` is added to the interface signature and passed from the
service layer. This is the narrowest fix: only `entry_fee` is re-verified;
the winner list from the ranker is treated as stable (match scores are immutable
once a match is finished).

---

## Alternatives considered

**Compute prize amounts inside the transaction:** Pass raw winner data (user IDs
+ KYC tiers) to `DistributePrizesAtomically` and compute amounts from the locked
`entry_fee`. Rejected as over-engineered for the actual risk — `entry_fee` is
the only mutable field on the distribution path; the winner list is derived from
immutable match scores.

**Optimistic lock with retry in the service layer:** Compare a version column
before and after the transaction. Rejected because it adds a retry loop with
no clear ceiling, and the FOR UPDATE approach is simpler with equivalent safety.

**Accept the race:** The probability of an admin changing `entry_fee` concurrently
with an active prize distribution is very low. Rejected because the consequence
(wrong prize amounts credited) is high-trust-cost even if low-probability.

---

## Consequences

- `DistributePrizesAtomically` now takes an `expectedEntryFee int` parameter.
  All callers (service layer + tests) must pass the entry_fee they read before
  the call.
- The `SELECT FOR UPDATE` adds one additional round-trip inside the transaction.
  This is negligible given that prize distribution is a rare, admin-only operation.

---

## Implementation

- `internal/repository/interfaces.go` — updated interface
- `internal/repository/quiniela_repository.go:DistributePrizesAtomically`
- `internal/service/admin_group_service.go:DistributePrizes`
- `internal/repository/quiniela_repository_test.go:TestQuinielaRepository_DistributePrizesAtomically_EntryFeeMismatch_ReturnsConflict`
