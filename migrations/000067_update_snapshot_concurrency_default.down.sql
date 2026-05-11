-- Revert worker.snapshot_concurrency default to 16.
UPDATE system_params
SET
    default_value = '16',
    value         = CASE WHEN value = '4' THEN '16' ELSE value END,
    updated_at    = NOW()
WHERE key = 'worker.snapshot_concurrency';
