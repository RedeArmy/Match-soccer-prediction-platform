-- Migration: make tiebreakers global (per-user, not per-group)
--
-- The tiebreaker question is now defined by the system administrator and
-- applies to every group uniformly. A user's numeric estimate is therefore
-- a single global prediction — there is no longer a per-quiniela dimension.
--
-- Steps:
--   1. Drop the per-group unique constraint and the quiniela_id FK.
--   2. Drop the per-row result column (result lives in tiebreaker_config).
--   3. Add a per-user unique constraint: each user may submit exactly one
--      global tiebreaker prediction.
ALTER TABLE tiebreakers
    DROP CONSTRAINT IF EXISTS uq_tiebreakers_user_quiniela,
    DROP COLUMN IF EXISTS quiniela_id,
    DROP COLUMN IF EXISTS result,
    ADD CONSTRAINT uq_tiebreakers_user UNIQUE (user_id);
