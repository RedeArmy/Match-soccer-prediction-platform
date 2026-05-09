-- Migration: add CHECK constraints on matches.group_label
--
-- Enforces two invariants that the application layer already validates but
-- that were previously unenforced at the database level, leaving a gap that
-- a direct SQL INSERT or a future bug could silently exploit.
--
-- Constraint 1 — valid domain values:
--   group_label must be NULL or exactly one uppercase letter in the range A–L
--   (12 groups, FIFA World Cup 2026 format). Values like "group_a", "Group A",
--   or "M" are rejected.
--
-- Constraint 2 — phase coherence:
--   group_stage matches must carry a group_label; all knockout matches must not.
--   This mirrors the invariant asserted by ValidateMatch() in the Go layer and
--   assumed by buildStandings() when grouping teams.
--
-- Both constraints use separate names so that a constraint-violation error
-- message clearly identifies which rule was broken.

ALTER TABLE matches
    ADD CONSTRAINT matches_group_label_valid
    CHECK (group_label IS NULL OR group_label ~ '^[A-L]$');

ALTER TABLE matches
    ADD CONSTRAINT matches_group_label_phase_coherent
    CHECK (
        (phase = 'group_stage' AND group_label IS NOT NULL)
        OR (phase <> 'group_stage' AND group_label IS NULL)
    );
