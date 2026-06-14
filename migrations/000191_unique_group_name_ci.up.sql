-- Replace the simple unique constraint on quinielas.name with a partial,
-- case-insensitive functional index. Benefits over the old constraint:
--   1. Case-insensitive: "Mi Grupo" and "mi grupo" are treated as the same name.
--   2. Soft-delete aware: deleted groups release their name so it can be reused.
ALTER TABLE quinielas DROP CONSTRAINT IF EXISTS quinielas_name_key;

CREATE UNIQUE INDEX idx_quinielas_name_ci
    ON quinielas (lower(name))
    WHERE deleted_at IS NULL;
