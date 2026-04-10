-- Migration: add soft-delete to users and quinielas
--
-- Hard DELETE on users or quinielas destroys payment and audit history via
-- ON DELETE CASCADE on group_memberships and predictions. Soft-delete
-- preserves the full history while hiding records from all application
-- queries; a future data-retention job can hard-delete rows past the
-- statutory retention period.
--
-- Partial indexes on NULL deleted_at keep read queries fast: the WHERE
-- clause in application queries matches the index predicate exactly, so
-- the planner uses the index rather than a full table scan.
ALTER TABLE users     ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE quinielas ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_users_active     ON users     (id) WHERE deleted_at IS NULL;
CREATE INDEX idx_quinielas_active ON quinielas (id) WHERE deleted_at IS NULL;
