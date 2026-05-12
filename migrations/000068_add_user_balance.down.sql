DROP INDEX IF EXISTS idx_users_balance;

ALTER TABLE users
  DROP COLUMN IF EXISTS reserved_cents,
  DROP COLUMN IF EXISTS balance_cents;
