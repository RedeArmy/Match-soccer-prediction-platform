-- session_starts records when each Clerk session was first seen by this server.
--
-- Background: Clerk refreshes short-lived JWTs every ~60 s. The JWT's iat (issued-at)
-- is always recent and cannot be used for session-age enforcement. Clerk embeds the
-- session origin in the fva[0] claim, but fva is absent for OAuth / social / automatic-
-- login sessions. When fva is absent, this table provides a stable origin timestamp:
-- PolicyProvider calls UpsertSessionStart on the first request from a given sid, which
-- either records the current time (new session) or returns the already-stored time
-- (returning session). This ensures max-age enforcement always measures from the true
-- session origin regardless of how the user authenticated.
--
-- Cleanup: rows older than auth.session_max_age_seconds + 1 day are removed by
-- the worker maintenance job ("session.starts_prune") that runs daily.
CREATE TABLE IF NOT EXISTS session_starts (
    sid        TEXT        PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index supports efficient pruning by time without a sequential scan.
CREATE INDEX IF NOT EXISTS idx_session_starts_started_at ON session_starts (started_at);
