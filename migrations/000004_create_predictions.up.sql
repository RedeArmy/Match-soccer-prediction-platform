-- Migration: create predictions table
--
-- The unique index on (user_id, match_id) is a database-level enforcement of
-- the invariant that each user may submit at most one prediction per match.
-- The application layer checks for an existing row before inserting, but this
-- index eliminates the race condition between the check and the insert.
CREATE TABLE IF NOT EXISTS predictions (
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    match_id   INTEGER     NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    home_score INTEGER     NOT NULL,
    away_score INTEGER     NOT NULL,
    points     INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_predictions_user_match UNIQUE (user_id, match_id)
);

CREATE INDEX IF NOT EXISTS idx_predictions_user_id  ON predictions (user_id);
CREATE INDEX IF NOT EXISTS idx_predictions_match_id ON predictions (match_id);
