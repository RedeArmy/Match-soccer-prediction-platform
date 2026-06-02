-- Migration 000152: enforce at most one active withdrawal request per user.
--
-- Problem: two concurrent POST /withdrawals for the same user could both pass
-- the available-balance check inside separate transactions before either had
-- incremented reserved_cents, resulting in a double-reservation or a
-- reservation that can never be committed.
--
-- Fix: a unique partial index on (user_id) scoped to rows whose status is
-- 'pending' or 'approved'. The INSERT in CreateAndReserve will raise SQLSTATE
-- 23505 for the second concurrent request; the repository maps this to
-- apperrors.Conflict so the HTTP handler returns 409.
--
-- This also makes the intent explicit as a business rule: users may hold at
-- most one pending withdrawal at a time, which is consistent with the manual
-- admin-review workflow.

CREATE UNIQUE INDEX IF NOT EXISTS withdrawal_requests_one_pending_per_user
    ON withdrawal_requests (user_id)
    WHERE status IN ('pending', 'approved');
