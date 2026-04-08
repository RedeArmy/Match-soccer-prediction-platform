-- Rollback: restore password_hash column
--
-- Restored as nullable (no NOT NULL constraint) because existing rows cannot
-- have their original hash values recovered after the column was dropped.
-- Forcing NOT NULL on rollback would make the rollback itself fail on any
-- row inserted while the column was absent. Accept NULL on rollback to keep
-- the schema structurally compatible without fabricating data.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
