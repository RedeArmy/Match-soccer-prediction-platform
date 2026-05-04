-- Add conflict.max_scan to system_params.
--
-- Caps the number of conflicts loaded into memory by ConflictSummary to prevent
-- unbounded memory growth when invoked by background jobs or dashboard widgets.
-- Under normal operation this limit should never be hit - 5000 unresolved conflicts
-- indicates a systemic operational issue. Admin endpoints that paginate conflict
-- lists (GET /admin/conflicts) are unaffected by this cap.
-- is_runtime = TRUE so the limit can be tuned without a process restart.

INSERT INTO system_params (key, value, type, category, is_runtime, description)
VALUES (
    'conflict.max_scan',
    '5000',
    'int',
    'conflict',
    true,
    'Maximum number of conflicts loaded into memory by ConflictSummary. Prevents OOM when conflict backlog is pathologically large.'
)
ON CONFLICT (key) DO NOTHING;
