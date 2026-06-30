-- Add user_id to session_starts to enable per-user session queries.
--
-- Use cases enabled:
--   • "Which sessions are active for user X?" (admin / support tooling)
--   • "Invalidate all sessions for a compromised account" via
--     DELETE FROM session_starts WHERE user_id = $1 (force logout all devices)
--   • Audit trail: session-start history attributable to a specific user
--
-- Nullable to preserve the existing rows inserted before this migration.
-- Application code always provides the user_id on new upserts.
ALTER TABLE session_starts ADD COLUMN IF NOT EXISTS user_id TEXT;

-- Supports efficient "find all sessions for user X" without a sequential scan.
CREATE INDEX IF NOT EXISTS idx_session_starts_user_id ON session_starts (user_id);
