-- Rename the system role value 'player' to 'user'.
--
-- 'player' implied group membership, but the role is assigned at account
-- creation — before any group exists. 'user' accurately describes what the
-- role means: an authenticated, non-admin account in the system.
--
-- Steps:
--   1. Migrate existing data so no row violates the new constraint.
--   2. Drop the anonymous CHECK from migration 000001.
--   3. Add a named CHECK with the new allowed values.
--   4. Update the column DEFAULT.
UPDATE users SET role = 'user' WHERE role = 'player';

ALTER TABLE users
    DROP CONSTRAINT users_role_check,
    ADD CONSTRAINT chk_users_role CHECK (role IN ('admin', 'user')),
    ALTER COLUMN role SET DEFAULT 'user';
