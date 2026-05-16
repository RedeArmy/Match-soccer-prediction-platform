-- Down migration: no-op.
--
-- Reverting a flag canonicalization migration is not meaningful: we cannot know
-- which rows had incorrect values before the up migration ran. The rows themselves
-- were seeded by earlier migrations (000051-000076) and must not be deleted here.
--
-- If a rollback is required, re-run the affected seed migration (e.g. 000051)
-- after running this down migration; it will re-apply the correct is_runtime values
-- for the base parameter set.
SELECT 1;
