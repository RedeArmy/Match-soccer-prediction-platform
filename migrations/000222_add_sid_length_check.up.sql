ALTER TABLE revoked_sessions
  ADD CONSTRAINT revoked_sessions_sid_length
    CHECK (char_length(sid) BETWEEN 1 AND 128);

ALTER TABLE session_starts
  ADD CONSTRAINT session_starts_sid_length
    CHECK (char_length(sid) BETWEEN 1 AND 128);
