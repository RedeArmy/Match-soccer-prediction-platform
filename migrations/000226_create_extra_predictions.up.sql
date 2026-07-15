-- extra_predictions: one row per user per match per extra type (e.g. "first
-- team to score", "half-time result"). Mirrors predictions' shape (upsert on
-- a unique key, nullable points until scored) but stores the guess as a
-- single TEXT column since different extra types have different answer
-- domains (home/away/none vs. home/draw/away) — validated at the service
-- layer rather than a single cross-type DB CHECK.
CREATE TABLE extra_predictions (
    id         SERIAL      PRIMARY KEY,
    user_id    INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    match_id   INTEGER     NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    extra_type TEXT        NOT NULL
               CHECK (extra_type IN ('first_scorer', 'halftime_result')),
    answer     TEXT        NOT NULL,
    points     INTEGER,
    scored_at  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_extra_predictions_user_match_type UNIQUE (user_id, match_id, extra_type)
);

-- Fast lookup for the match-list view: "all of my extra guesses for these matches".
CREATE INDEX ON extra_predictions (user_id, match_id);

-- Fast lookup for the scoring batch: "all guesses for this match, this type".
CREATE INDEX ON extra_predictions (match_id);
