-- Rollback: restore membership_status ENUM and revert the status column.
--
-- Pre-flight check: the cast status::membership_status below succeeds only
-- if every row holds one of the three original values. If a later migration
-- (or direct write) introduced an additional status value the cast would fail
-- mid-transaction, leaving the table partially modified. The block below
-- aborts with a clear message before any structural change is made.
DO $$
DECLARE
    bad_values TEXT;
BEGIN
    SELECT string_agg(DISTINCT status, ', ' ORDER BY status)
      INTO bad_values
      FROM group_memberships
     WHERE status NOT IN ('pending', 'active', 'left');

    IF bad_values IS NOT NULL THEN
        RAISE EXCEPTION
            'Cannot roll back migration 000017: group_memberships.status '
            'contains value(s) [%] that are not members of the original '
            'membership_status enum. Remove or remap those rows before retrying.',
            bad_values;
    END IF;
END;
$$;

CREATE TYPE membership_status AS ENUM ('pending', 'active', 'left');

ALTER TABLE group_memberships
    ADD COLUMN status_enum membership_status NOT NULL DEFAULT 'pending';

UPDATE group_memberships SET status_enum = status::membership_status;

-- DROP COLUMN also drops the group_memberships_status_check constraint.
ALTER TABLE group_memberships DROP COLUMN status;

ALTER TABLE group_memberships RENAME COLUMN status_enum TO status;
