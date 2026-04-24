-- Add removal audit columns to group_memberships for complete soft-delete traceability.
--
-- Previously the only signal that a membership was removed was status = 'left',
-- which does not distinguish between a voluntary exit and an admin-forced removal.
--
-- removed_at records the timestamp of the status → 'left' transition.
-- removed_by records the user_id of the actor who triggered the removal:
--   NULL  → the member left voluntarily (self-exit via Leave)
--   <id>  → an administrator removed the member (via RemoveByAdmin)
--
-- Both columns are NULL for memberships that have never transitioned to 'left'
-- (status = 'active' or 'pending') and for historical rows created before this
-- migration.
ALTER TABLE group_memberships
    ADD COLUMN removed_at TIMESTAMPTZ,
    ADD COLUMN removed_by INT REFERENCES users(id) ON DELETE SET NULL;
