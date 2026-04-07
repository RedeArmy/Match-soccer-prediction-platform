DROP INDEX IF EXISTS idx_users_clerk_subject;
ALTER TABLE users DROP COLUMN IF EXISTS clerk_subject;
