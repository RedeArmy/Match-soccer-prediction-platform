-- Add admin.bulk_max_items to system_params.
--
-- This param caps the number of IDs accepted by bulk admin operations
-- (BulkDeleteGroups, BulkRemoveMembers). It is read dynamically on every
-- request (is_runtime = TRUE) so it can be lowered during high-load periods
-- or raised for a planned mass-cleanup without a process restart.
--
-- Default: 1000 — large enough for practical admin use while preventing
-- oversized ANY($1) queries that would stress the database.

INSERT INTO system_params (key, value, type, category, is_runtime)
VALUES ('admin.bulk_max_items', '1000', 'int', 'admin', TRUE)
ON CONFLICT (key) DO NOTHING;
