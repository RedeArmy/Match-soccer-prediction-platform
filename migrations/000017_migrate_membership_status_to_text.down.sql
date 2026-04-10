-- Rollback: restore membership_status ENUM and revert the status column.
CREATE TYPE membership_status AS ENUM ('pending', 'active', 'left');

ALTER TABLE group_memberships
    ADD COLUMN status_enum membership_status NOT NULL DEFAULT 'pending';

UPDATE group_memberships SET status_enum = status::membership_status;

ALTER TABLE group_memberships DROP COLUMN status;

ALTER TABLE group_memberships RENAME COLUMN status_enum TO status;
