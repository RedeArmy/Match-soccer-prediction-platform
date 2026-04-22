DROP INDEX IF EXISTS idx_users_banned_at;

ALTER TABLE users
    DROP COLUMN IF EXISTS ban_reason,
    DROP COLUMN IF EXISTS banned_by,
    DROP COLUMN IF EXISTS banned_at;
