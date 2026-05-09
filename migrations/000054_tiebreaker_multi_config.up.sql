-- Extend tiebreaker_config so administrators can create phase-scoped or
-- group-scoped questions in addition to the existing platform-wide singleton.
--
-- Design:
--   phase IS NULL AND quiniela_id IS NULL  → global (the original singleton, id=1)
--   phase IS NOT NULL AND quiniela_id IS NULL → per-tournament-phase
--   phase IS NULL AND quiniela_id IS NOT NULL → per-quiniela
--   phase IS NOT NULL AND quiniela_id IS NOT NULL → per-quiniela AND per-phase
--
-- Partial unique indices prevent duplicate configurations per scope while
-- correctly handling NULL values (NULL != NULL in standard unique constraints).
--
-- Existing data: the id=1 global row gets phase=NULL, quiniela_id=NULL, which
-- is its correct classification. No data migration is needed.

ALTER TABLE tiebreaker_config
    ADD COLUMN phase       TEXT,
    ADD COLUMN quiniela_id INT REFERENCES quinielas(id) ON DELETE CASCADE;

-- At most one global config (both phase and quiniela_id are NULL).
-- The constant-key trick (((true))) forces all matching rows into the same
-- index bucket, so PostgreSQL rejects a second global insert as a duplicate.
CREATE UNIQUE INDEX uq_tiebreaker_config_global
    ON tiebreaker_config (((true)))
    WHERE phase IS NULL AND quiniela_id IS NULL;

-- At most one config per tournament phase (platform-wide, no quiniela scope).
CREATE UNIQUE INDEX uq_tiebreaker_config_phase
    ON tiebreaker_config (phase)
    WHERE phase IS NOT NULL AND quiniela_id IS NULL;

-- At most one config per quiniela (group-specific, no phase scope).
CREATE UNIQUE INDEX uq_tiebreaker_config_quiniela
    ON tiebreaker_config (quiniela_id)
    WHERE quiniela_id IS NOT NULL AND phase IS NULL;

-- At most one config per (quiniela, phase) pair.
CREATE UNIQUE INDEX uq_tiebreaker_config_quiniela_phase
    ON tiebreaker_config (quiniela_id, phase)
    WHERE quiniela_id IS NOT NULL AND phase IS NOT NULL;

-- Link each user prediction to the specific tiebreaker question it answers.
-- DEFAULT 1 maps all existing rows to the global config (id=1), preserving
-- full backward compatibility with no data migration required.
ALTER TABLE tiebreakers
    ADD COLUMN tiebreaker_config_id INT NOT NULL DEFAULT 1
        REFERENCES tiebreaker_config(id) ON DELETE CASCADE;

-- Replace the global per-user unique constraint with a per-config one,
-- allowing a user to submit one prediction for each active config.
ALTER TABLE tiebreakers
    DROP CONSTRAINT uq_tiebreakers_user;

ALTER TABLE tiebreakers
    ADD CONSTRAINT uq_tiebreakers_user_config UNIQUE (user_id, tiebreaker_config_id);
