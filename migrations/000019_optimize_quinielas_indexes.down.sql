-- Revert quinielas index optimizations.
--
-- Restore the original single-column owner index and drop the additions
-- introduced in the up migration.

DROP INDEX IF EXISTS idx_quinielas_invite_code;

DROP INDEX IF EXISTS idx_quinielas_owner_created;

CREATE INDEX IF NOT EXISTS idx_quinielas_owner_id
    ON quinielas (owner_id);
