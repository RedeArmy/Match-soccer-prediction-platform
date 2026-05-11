-- Migration: update worker.snapshot_concurrency default from 16 to 4
--
-- DefaultWorkerSnapshotConcurrency was reduced from 16 to 4 to prevent
-- OOM crashes on 256 MB Fly.io machines. With 16 concurrent goroutines each
-- holding pgx row buffers during ranking queries, peak heap usage can exceed
-- the 256 MB machine limit under match-day load.
--
-- This migration syncs the default_value column (the seed default used by
-- validate-params and service fallbacks) with the new domain constant.
-- The live value column is updated only when it still matches the old default
-- (16), preserving any operator override above 16 that may have been set for
-- a larger instance.
--
-- Idempotent: safe to re-run; DO UPDATE never changes value if already 4.

UPDATE system_params
SET
    default_value = '4',
    value         = CASE WHEN value = '16' THEN '4' ELSE value END,
    updated_at    = NOW()
WHERE key = 'worker.snapshot_concurrency';
