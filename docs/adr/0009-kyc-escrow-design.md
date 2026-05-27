# ADR 0009 – KYC escrow design (prize freeze)

**Status:** Accepted  
**Date:** 2026-05-26  
**Deciders:** Engineering team  
**Context:** Guatemala SIB/UAF AML regulations, KYC/AML module (migrations 000116–000121)

---

## Context

Guatemalan UAF regulations require that operators withhold prize disbursements to
users who have not reached KYC Tier 2 (identity verified + address verified) when
the prize amount exceeds the AML reporting threshold.  The funds must not be lost —
they must be held in escrow and credited to the user once their KYC review passes.

The system also needs an immutable audit trail that connects the freeze event, the
admin review decision, and the eventual credit.

---

## Decision

### Where frozen funds are stored

Frozen prize amounts are held in `kyc_profiles.frozen_amount_cents`.  The companion
column `kyc_profiles.balance_frozen = TRUE` signals that a freeze is active.

**Rationale for this location (not a separate escrow table):**

- The freeze is 1:1 with a KYC profile.  A user can be in at most one frozen-prize
  state at a time (the AML policy covers the aggregate, not per-prize).
- Keeping the amount on the profile row lets `ReleaseAndCreditFrozen` acquire a
  single `SELECT … FOR UPDATE` row lock, eliminating race conditions between
  concurrent admin approval and re-freeze events without needing a separate escrow
  table.
- Adds zero additional tables; the KYC profile is already the authoritative record
  for the user's compliance state.

### What triggers the freeze

`PrizeCrediter.CreditPrize` calls `KYCGate.CheckWinFreeze` before writing any ledger
row.  If the gate returns `shouldFreeze=true` (user below KYCTierTwo), the service:

1. Calls `KYCService.FreezeBalance`, which sets `balance_frozen=TRUE` and records
   `frozen_amount_cents` on the `kyc_profiles` row.
2. Writes a `kyc.winner_freeze` event to the transactional outbox so the n8n
   workflow notifies the user and flags the profile for admin review.
3. Returns `(credited=false, err=nil)` to the caller — the prize is not lost; it
   is held.

The caller (`adminGroupService.DistributePrizes`) treats `credited=false` as a
successful escrow and continues distributing prizes to other winners.

### What triggers the credit

An admin approves the KYC profile via `POST /admin/kyc/profiles/:id/approve`.  The
handler calls `KYCService.Approve`, which (among other things) sets `users.kyc_tier`
to the approved tier.

Separately, `KYCService.ReleaseFrozenBalance` is called (either manually by an admin
or by the `RecalculateRiskScore` job after tier promotion).  It delegates to
`KYCProfileRepository.ReleaseAndCreditFrozen`, which runs atomically inside a single
database transaction:

```
BEGIN;
  SELECT id, frozen_amount_cents FROM kyc_profiles
    WHERE user_id=$1 AND balance_frozen=TRUE FOR UPDATE;   -- row lock
  UPDATE users SET balance_cents = balance_cents + $frozen_amount
    WHERE id=$1 RETURNING balance_cents;                   -- credit
  INSERT INTO balance_ledger (...) VALUES (...);           -- audit row
  UPDATE kyc_profiles
    SET balance_frozen=FALSE, frozen_amount_cents=0,
        frozen_reason='', updated_at=NOW()
    WHERE user_id=$1;                                      -- clear escrow
COMMIT;
```

If the profile is not frozen the method returns `(0, nil)` without writing anything —
the call is idempotent.

### Audit trail

Every state transition writes a `kyc_events` row via `appendEvent` (best-effort,
outside the transaction) and an `audit_log` entry via `AuditService.Log`.  The
`balance_ledger` row written by `ReleaseAndCreditFrozen` carries `ref_type='kyc_unfreeze'`
and `ref_id=kyc_profiles.id`, providing a permanent link between the escrow release
and the originating KYC profile.

---

## Consequences

- **Positive:** The freeze/release cycle is atomic.  There is no window in which
  `balance_frozen=TRUE` and `balance_cents` have both been updated (or neither).
  Double-release is safe: the idempotency guard returns early on the second call.
- **Positive:** No additional table or sequence needed; the existing KYC profile
  row carries all state required for the escrow.
- **Negative:** Only one prize freeze per user is supported simultaneously.  If a
  second qualifying prize arrives while the first is frozen, `FreezeBalance` will
  overwrite (increase) `frozen_amount_cents`.  This is intentional — UAF regulation
  treats the cumulative withheld amount, not individual prizes.
- **Deferred:** If per-prize traceability is ever required by regulators, a separate
  `kyc_frozen_prizes` table should be introduced.  The current column approach would
  then become a denormalised cache on the profile row.
