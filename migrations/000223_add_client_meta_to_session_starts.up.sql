-- Add client attribution columns to session_starts for forensic traceability.
--
-- When suspicious account activity is detected, investigators need to answer
-- "which IP and browser started this session?" session_starts is the only table
-- that maps a Clerk sid to a first-seen timestamp for OAuth/fva-absent flows.
-- Without IP and user-agent, that forensic path is blocked.
--
-- Both columns are nullable so that rows inserted before this migration (or by
-- internal/headless paths with no HTTP context) remain valid. Application code
-- always provides the values from the request context on new upserts.
ALTER TABLE session_starts
  ADD COLUMN IF NOT EXISTS client_ip   TEXT,
  ADD COLUMN IF NOT EXISTS user_agent  TEXT;
