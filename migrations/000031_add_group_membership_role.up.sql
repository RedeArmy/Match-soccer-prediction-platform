ALTER TABLE group_memberships
    ADD COLUMN role TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('member', 'owner'));

-- Backfill: memberships that belong to the group owner become 'owner'.
-- group_memberships has no soft-delete column; filter on quinielas instead.
UPDATE group_memberships gm
    SET role = 'owner'
   FROM quinielas q
  WHERE gm.quiniela_id = q.id
    AND gm.user_id     = q.owner_id
    AND q.deleted_at IS NULL;
