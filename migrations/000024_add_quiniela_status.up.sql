-- Add status column to quinielas.
--
-- A quiniela is considered "active" when it has at least MinMembersForActive
-- (3) active members. Groups with fewer members are "inactive": predictions
-- can still be submitted but the group is not eligible for payment processing
-- or prize distribution.
--
-- Status is managed exclusively by the system via syncGroupStatus, which is
-- triggered on every membership transition (ApproveJoin, Leave). No HTTP
-- endpoint exposes a direct status change.
--
-- The partial index on (status) WHERE deleted_at IS NULL makes it cheap to
-- filter active groups without scanning soft-deleted rows.

ALTER TABLE quinielas
    ADD COLUMN status TEXT NOT NULL DEFAULT 'inactive'
        CONSTRAINT quinielas_status_valid CHECK (status IN ('active', 'inactive'));

-- Back-fill: groups that already have 3 or more active members are active.
UPDATE quinielas q
   SET status = 'active'
 WHERE q.deleted_at IS NULL
   AND (
         SELECT COUNT(*)
           FROM group_memberships gm
          WHERE gm.quiniela_id = q.id
            AND gm.status = 'active'
       ) >= 3;

CREATE INDEX idx_quinielas_status ON quinielas(status) WHERE deleted_at IS NULL;
