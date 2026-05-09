-- Track which match event triggered each leaderboard snapshot.
-- The partial unique index makes worker-triggered snapshots idempotent:
-- replaying the same MatchFinished event for the same quiniela converges to
-- the same snapshot row instead of creating a duplicate.
-- Admin-triggered snapshots (triggered_by_match_id IS NULL) are exempt and
-- may accumulate multiple rows (each manual recalculation is intentional).
ALTER TABLE leaderboard_snapshots
    ADD COLUMN triggered_by_match_id INTEGER REFERENCES matches(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX uq_leaderboard_snapshots_quiniela_match
    ON leaderboard_snapshots (quiniela_id, triggered_by_match_id)
    WHERE triggered_by_match_id IS NOT NULL;
