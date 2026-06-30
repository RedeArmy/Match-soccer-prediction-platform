DROP INDEX IF EXISTS idx_session_starts_user_id;
ALTER TABLE session_starts DROP COLUMN IF EXISTS user_id;
