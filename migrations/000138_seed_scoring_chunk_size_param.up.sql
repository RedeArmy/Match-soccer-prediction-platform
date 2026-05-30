-- Seeds scoring.update_chunk_size: the maximum number of prediction rows updated
-- in a single UNNEST UPDATE batch inside ScoreMatchBatch (ATD-003).
-- Default 500 is safe for any Fly.io machine size; operators can lower it under
-- memory pressure or raise it on larger DB instances to reduce round-trips.
-- is_runtime=FALSE: the worker reads this value once per ScoreMatch invocation;
-- a process restart is required to pick up a changed value.
INSERT INTO system_params (key, value, default_value, type, category, description, is_runtime)
VALUES (
    'scoring.update_chunk_size',
    '500',
    '500',
    'int',
    'scoring',
    'Maximum number of prediction rows updated in a single UNNEST UPDATE batch during match scoring. Lower under memory pressure; raise to reduce round-trips on large matches.',
    FALSE
)
ON CONFLICT (key) DO NOTHING;
