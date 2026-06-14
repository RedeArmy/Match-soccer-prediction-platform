DROP INDEX IF EXISTS idx_quinielas_name_ci;

ALTER TABLE quinielas ADD CONSTRAINT quinielas_name_key UNIQUE (name);
