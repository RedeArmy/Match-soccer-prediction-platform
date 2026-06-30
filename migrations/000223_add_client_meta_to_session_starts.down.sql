ALTER TABLE session_starts
  DROP COLUMN IF EXISTS client_ip,
  DROP COLUMN IF EXISTS user_agent;
