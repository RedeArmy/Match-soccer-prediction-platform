ALTER TABLE revoked_sessions
  DROP CONSTRAINT IF EXISTS revoked_sessions_sid_length;

ALTER TABLE session_starts
  DROP CONSTRAINT IF EXISTS session_starts_sid_length;
