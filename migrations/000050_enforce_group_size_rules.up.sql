-- Migration: enforce fixed group size rules
--
-- Business rules now require:
--   • Minimum  5 active members for prize eligibility (was 3).
--   • Maximum 20 active members per group (hard platform cap).
--   • Winner count is determined by a fixed tier table, not a per-group
--     prize_threshold ratio. The prize_threshold and max_members columns
--     on quinielas are retired; their DB enforcement is replaced here.
--
-- This migration:
--   1. Replaces the dynamic enforce_max_members trigger with a fixed cap of 20.
--   2. Updates the group.min_members_for_active system param from 3 → 5.
--   3. Removes the group.default_prize_threshold system param (no longer used).

-- 1. Replace trigger function: always enforce 20, no longer reads quinielas.max_members.
CREATE OR REPLACE FUNCTION enforce_max_members()
RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    active_count INT;
BEGIN
    -- Only enforce when a row becomes active.
    IF NEW.status != 'active' THEN
        RETURN NEW;
    END IF;
    -- For UPDATE active→active, the count does not change.
    IF TG_OP = 'UPDATE' AND OLD.status = 'active' THEN
        RETURN NEW;
    END IF;

    -- BEFORE trigger: the row being inserted/updated is not yet visible.
    SELECT COUNT(*) INTO active_count
      FROM group_memberships
     WHERE quiniela_id = NEW.quiniela_id
       AND status = 'active';

    IF active_count >= 20 THEN
        RAISE EXCEPTION 'max_members_exceeded';
    END IF;

    RETURN NEW;
END;
$$;

-- 2. Update the runtime system param so GroupMembershipService picks up 5 immediately.
UPDATE system_params
   SET value      = '5',
       updated_at = NOW()
 WHERE key = 'group.min_members_for_active';

-- 3. Remove the retired prize_threshold param.
DELETE FROM system_params WHERE key = 'group.default_prize_threshold';
