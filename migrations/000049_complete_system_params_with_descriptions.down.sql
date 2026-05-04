-- This migration is additive (adds descriptions only).
-- Rolling back would remove descriptions, which is not desired.
-- A no-op rollback is safer than removing metadata.

-- If rollback is truly needed, manually run:
-- UPDATE system_params SET description = NULL WHERE description IS NOT NULL;
