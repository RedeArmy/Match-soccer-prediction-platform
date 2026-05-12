-- Add balance tracking columns to users.
--
-- balance_cents  : spendable funds in the account's minor currency unit (e.g. centavos).
--                  Must never go negative; enforced by CHECK and by the
--                  DebitBalance / CommitReservation repository methods which
--                  use a conditional UPDATE (WHERE balance_cents - $delta >= 0).
-- reserved_cents : funds locked for a pending withdrawal request; deducted from
--                  available balance until the request is approved or rejected.
--                  available = balance_cents - reserved_cents.
--
-- Both default to 0 so the migration is safe for existing user rows.

ALTER TABLE users
  ADD COLUMN balance_cents  INT NOT NULL DEFAULT 0 CHECK (balance_cents  >= 0),
  ADD COLUMN reserved_cents INT NOT NULL DEFAULT 0 CHECK (reserved_cents >= 0);

CREATE INDEX idx_users_balance ON users (balance_cents) WHERE deleted_at IS NULL;
