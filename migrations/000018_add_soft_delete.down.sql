DROP INDEX IF EXISTS idx_quinielas_active;
DROP INDEX IF EXISTS idx_users_active;

ALTER TABLE quinielas DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE users     DROP COLUMN IF EXISTS deleted_at;
