-- Migration: add snapshot.keep_latest_count system parameter
--
-- Bounds leaderboard_snapshots table growth by configuring how many recent
-- snapshots the daily purge job retains per quiniela. Without this cap the
-- table grows by (active_quinielas) rows per match scored; with 64 matches
-- and 100 active quinielas that is 6 400 rows per tournament — and grows
-- indefinitely across tournaments.
--
-- Default of 5 retains the last five match snapshots per quiniela, which is
-- enough for trend display and dashboard charts while keeping the table small.
-- is_runtime=FALSE: the worker reads this once at startup; a restart is needed
-- to apply a new value.

INSERT INTO system_params (key, value, type, category, is_runtime, description) VALUES
    ('snapshot.keep_latest_count', '5', 'int', 'worker', FALSE,
     'Number of most-recent leaderboard snapshots to retain per quiniela. The daily purge job deletes every snapshot beyond this count, bounding table growth to active_quinielas × keep_latest_count rows.')
ON CONFLICT (key) DO UPDATE SET
    value       = EXCLUDED.value,
    type        = EXCLUDED.type,
    category    = EXCLUDED.category,
    is_runtime  = EXCLUDED.is_runtime,
    description = EXCLUDED.description,
    updated_at  = NOW();
