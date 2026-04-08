-- Add payment tracking to group memberships.
--
-- paid is set to TRUE automatically when a quiniela has entry_fee = 0 (free
-- group). For paid groups (entry_fee > 0) it starts FALSE and is flipped to
-- TRUE by the payment system once the transfer is confirmed — never by a
-- human admin directly.
--
-- Only members with paid = TRUE are included in leaderboard calculations.
-- Members with paid = FALSE can still submit predictions; their scores are
-- excluded from rankings until the payment system confirms the transaction.

ALTER TABLE group_memberships
    ADD COLUMN paid BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill: mark existing memberships as paid for free groups.
UPDATE group_memberships gm
SET paid = TRUE
FROM quinielas q
WHERE gm.quiniela_id = q.id
  AND q.entry_fee = 0;
