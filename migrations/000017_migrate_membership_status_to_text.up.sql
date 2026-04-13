-- Migration: replace membership_status ENUM with TEXT + CHECK constraint
--
-- All other status columns in this schema (match status, user role) use
-- TEXT + CHECK, which allows adding new values with a simple ALTER TABLE
-- without holding an ACCESS EXCLUSIVE lock or touching the type catalog.
-- The membership_status ENUM introduced in 000013 is inconsistent with
-- this pattern and is replaced here.
--
-- The column swap is done in four steps to stay safe under concurrent load:
--   1. Add the new TEXT column with a named CHECK constraint, defaulting to
--      'pending'. The constraint is named explicitly so it survives the
--      column rename with a predictable, referenceable name.
--   2. Back-fill existing rows by casting the enum value to TEXT.
--   3. Drop the old ENUM column.
--   4. Rename the new column back to 'status'.
-- Only after step 4 is the DROP TYPE issued; dropping it earlier would block
-- the cast in step 2.
ALTER TABLE group_memberships
    ADD COLUMN status_text TEXT NOT NULL DEFAULT 'pending'
        CONSTRAINT group_memberships_status_check
        CHECK (status_text IN ('pending', 'active', 'left'));

UPDATE group_memberships SET status_text = status::TEXT;

ALTER TABLE group_memberships DROP COLUMN status;

ALTER TABLE group_memberships RENAME COLUMN status_text TO status;

DROP TYPE membership_status;
