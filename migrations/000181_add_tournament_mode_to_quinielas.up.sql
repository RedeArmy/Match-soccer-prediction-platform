-- Migration 000181: add hybrid tournament-mode columns to quinielas.
--
-- is_premium  — false (free, points-only; no prize distribution) or
--               true  (paid modes active; at least one of mode_general /
--               mode_round must be true when is_premium=true).
--
-- mode_general — when true, members pay a single entry fee to compete in the
--               overall-competition prize pool. Prize distribution follows the
--               existing WinnerCount tier table.
--
-- mode_round   — when true, members pay per jornada/round. Only members who
--               have a quiniela_round_entries row for the active round are
--               included in that round's prize distribution (1st + 2nd only).
--
-- Backwards-compatibility backfill: existing rows with entry_fee > 0 are
-- treated as having already been operating in general premium mode.

ALTER TABLE quinielas
    ADD COLUMN is_premium   BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN mode_general BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN mode_round   BOOLEAN NOT NULL DEFAULT FALSE;

-- Any group that was already charging an entry fee was effectively a general-mode
-- premium group; preserve that intent for existing data.
UPDATE quinielas
SET is_premium   = TRUE,
    mode_general = TRUE
WHERE entry_fee > 0
  AND deleted_at IS NULL;

COMMENT ON COLUMN quinielas.is_premium   IS 'true = at least one paid mode active; false = free (points only, no prizes)';
COMMENT ON COLUMN quinielas.mode_general IS 'prize pool for the overall competition winner(s); entry_fee charged once on join';
COMMENT ON COLUMN quinielas.mode_round   IS 'prize pool per jornada/round; quiniela_round_entries tracks per-round payment';
