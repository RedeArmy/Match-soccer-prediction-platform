-- Migration: create tiebreakers table
--
-- The unique index on (user_id, quiniela_id) mirrors the prediction uniqueness
-- constraint: one tiebreaker forecast per player per quiniela.
CREATE TABLE IF NOT EXISTS tiebreakers (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    quiniela_id INTEGER     NOT NULL REFERENCES quinielas(id) ON DELETE CASCADE,
    prediction  INTEGER     NOT NULL,
    result      INTEGER,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tiebreakers_user_quiniela UNIQUE (user_id, quiniela_id)
);
