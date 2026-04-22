CREATE TABLE leaderboard_snapshots (
    id          BIGSERIAL   PRIMARY KEY,
    quiniela_id INT         NOT NULL REFERENCES quinielas (id) ON DELETE CASCADE,
    taken_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    entries     JSONB       NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_leaderboard_snapshots_quiniela_id ON leaderboard_snapshots (quiniela_id);
-- Enables efficient "latest snapshot" queries per group.
CREATE INDEX idx_leaderboard_snapshots_taken_at    ON leaderboard_snapshots (quiniela_id, taken_at DESC);
