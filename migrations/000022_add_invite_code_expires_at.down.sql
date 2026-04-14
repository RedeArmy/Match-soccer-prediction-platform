DROP INDEX IF EXISTS idx_quinielas_invite_code_active;

-- Restore the single-column index from migration 000019.
CREATE INDEX idx_quinielas_invite_code
    ON quinielas (invite_code)
    WHERE deleted_at IS NULL;

ALTER TABLE quinielas
    DROP COLUMN IF EXISTS invite_code_expires_at;
