-- Local session revocation table.
--
-- Clerk controls the cryptographic lifetime of JWTs, but the application can
-- enforce its own session policies independently by maintaining a blocklist of
-- revoked session IDs (the "sid" claim present in every Clerk JWT).
--
-- Two enforcement mechanisms use this table:
--   1. iat-based max-age: checked in-process (no I/O) using auth.session_max_age_seconds.
--   2. Explicit revocation: POST /api/v1/auth/logout inserts a row here; subsequent
--      requests carrying the same sid are rejected with 401 even if the JWT is still
--      cryptographically valid and within Clerk's own session lifetime.
--
-- Rows are pruned by the worker's "session.revoked_prune" scheduler job once they
-- are older than max_age + 1 day (entries beyond max_age cannot match a live token
-- because the iat check would already have rejected it).

CREATE TABLE revoked_sessions (
    sid        TEXT        NOT NULL,
    user_id    TEXT        NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT revoked_sessions_pkey PRIMARY KEY (sid)
);

-- Index to support the periodic pruning query (DELETE WHERE revoked_at < $1).
CREATE INDEX idx_revoked_sessions_revoked_at ON revoked_sessions (revoked_at);

-- Seed the session max-age system parameter.
-- 604800 seconds = 7 days, matching Clerk's default free-tier session lifetime.
-- is_runtime=TRUE: changes propagate within 30 s without a process restart.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'auth.session_max_age_seconds',
    '604800',
    '604800',
    'int',
    'auth',
    TRUE,
    'Maximum session lifetime in seconds enforced by the application layer independently of Clerk. Tokens whose iat is older than this value are rejected with 401. Default: 604800 (7 days). is_runtime=TRUE: changes propagate within 30 s without restart.'
) ON CONFLICT (key) DO NOTHING;
