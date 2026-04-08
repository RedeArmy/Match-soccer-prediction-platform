-- Migration: add phase column to matches
--
-- phase tracks which round of the tournament a fixture belongs to.
-- FIFA World Cup 2026 expands to 48 teams, introducing a new round_of_32
-- between the group stage and the traditional round_of_16.
--
-- The column is NOT NULL with a default of 'group_stage' so that existing
-- rows (created before this migration) are assigned the most common phase
-- without requiring a backfill script.
ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS phase TEXT NOT NULL DEFAULT 'group_stage'
        CHECK (phase IN (
            'group_stage',
            'round_of_32',
            'round_of_16',
            'quarter_final',
            'semi_final',
            'third_place',
            'final'
        ));

CREATE INDEX IF NOT EXISTS matches_phase_idx ON matches (phase);
