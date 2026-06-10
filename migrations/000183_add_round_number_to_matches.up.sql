-- Migration 000183: add round_number to matches for per-round leaderboard support.
--
-- round_number groups matches into discrete jornadas/rounds for the hybrid
-- tournament feature. It drives both the per-round leaderboard breakdown and
-- the per-round prize distribution in mode_round groups.
--
-- Mapping (FIFA 2026):
--   1 — Group-stage matchday 1 (first 2 games per group, ordered by kickoff_at)
--   2 — Group-stage matchday 2
--   3 — Group-stage matchday 3
--   4 — Round of 32
--   5 — Round of 16
--   6 — Quarter-finals
--   7 — Semi-finals
--   8 — Third-place play-off
--   9 — Final
--
-- Group-stage round numbers are derived from ROW_NUMBER() ordered by kickoff_at
-- within each group label. Because each group plays exactly 6 fixtures across 3
-- matchdays (2 per matchday), rows 1–2 map to round 1, 3–4 to round 2,
-- and 5–6 to round 3. The secondary sort by id is stable across equal kickoffs.

ALTER TABLE matches ADD COLUMN round_number INT;

-- Backfill group-stage matchdays (round 1-3 per group_label).
WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY group_label
            ORDER BY kickoff_at, id
        ) AS rn
    FROM matches
    WHERE phase = 'group_stage'
      AND group_label IS NOT NULL
)
UPDATE matches m
SET round_number = CASE WHEN r.rn <= 2 THEN 1
                        WHEN r.rn <= 4 THEN 2
                        ELSE 3
                   END
FROM ranked r
WHERE m.id = r.id;

-- Backfill knockout phases with fixed round numbers.
UPDATE matches SET round_number = 4 WHERE phase = 'round_of_32';
UPDATE matches SET round_number = 5 WHERE phase = 'round_of_16';
UPDATE matches SET round_number = 6 WHERE phase = 'quarter_final';
UPDATE matches SET round_number = 7 WHERE phase = 'semi_final';
UPDATE matches SET round_number = 8 WHERE phase = 'third_place';
UPDATE matches SET round_number = 9 WHERE phase = 'final';

COMMENT ON COLUMN matches.round_number IS
    '1–3 = group-stage matchdays; 4=R32, 5=R16, 6=QF, 7=SF, 8=3rd, 9=final';
