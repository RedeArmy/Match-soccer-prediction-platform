-- Migration 000184: add group.free_max_members system param
-- Maximum number of active members allowed in a free (non-premium) group.
-- When this limit is reached, new join requests and approvals are blocked
-- until the group is upgraded to premium.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'group.free_max_members',
    '5',
    '5',
    'int',
    'group',
    true,
    'Maximum active members allowed in a free (non-premium) group. Joining via invite code and approving pending members are both blocked once this limit is reached. Upgrade the group to premium to allow more members.'
)
ON CONFLICT (key) DO NOTHING;
