-- Migration: remove enforce_max_members trigger
--
-- The trg_enforce_max_members trigger was the only enforcement point for the
-- group size cap, and it hardcoded the limit at 20. system_params already
-- carries a group.max_size key that administrators can change at runtime, but
-- the trigger silently ignored it — creating two conflicting sources of truth.
--
-- This migration:
--   1. Drops the trigger and its backing function entirely.
--   2. Seeds group.max_size = 20 into system_params so the application layer
--      has a single, authoritative, runtime-configurable source for the cap.
--
-- After this migration the capacity check is performed exclusively inside
-- RequestJoinByInviteCode and ApproveMembership, both of which read the limit
-- from system_params (falling back to domain.MaxMembersPerGroup when absent).
-- Both operations hold a FOR UPDATE lock on the quiniela row for the duration
-- of the check-then-write, preserving the race-safety guarantee previously
-- provided by the trigger.

DROP TRIGGER IF EXISTS trg_enforce_max_members ON group_memberships;
DROP FUNCTION IF EXISTS enforce_max_members();

INSERT INTO system_params (key, value, type, category, description, is_runtime)
VALUES (
    'group.max_size',
    '20',
    'int',
    'group',
    'Maximum number of active members allowed per quiniela. Enforced by the application layer on join and approval. Changing this value takes effect immediately without a process restart.',
    TRUE
)
ON CONFLICT (key) DO UPDATE
    SET value       = EXCLUDED.value,
        description = EXCLUDED.description,
        updated_at  = NOW();
