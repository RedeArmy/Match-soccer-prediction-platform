-- Adds a non-negative CHECK on balance_ledger.balance_after.
--
-- balance_after is written by the application after reading the post-mutation
-- value from users.balance_cents (which already has its own CHECK >= 0), so
-- the constraint should never fire under normal operation. Its value is to catch
-- calculation bugs or accidental direct writes before they corrupt the audit trail.
--
-- NOT VALID allows the constraint to be added without a full table scan (and
-- its associated long-held lock). New and updated rows are validated immediately.
-- The subsequent VALIDATE runs under ShareUpdateExclusiveLock, which allows
-- concurrent reads and writes while scanning existing rows.
ALTER TABLE balance_ledger
  ADD CONSTRAINT chk_balance_after_non_negative CHECK (balance_after >= 0) NOT VALID;

ALTER TABLE balance_ledger VALIDATE CONSTRAINT chk_balance_after_non_negative;
