-- Migration: seed missing group.max_size system parameter
--
-- group.max_size was introduced as ParamKeyGroupMaxSize in domain/constants.go
-- and is read at request-time by GroupMembershipService (RequestJoinByInviteCode
-- and ApproveMembership) to enforce the per-quiniela member cap. However, the
-- corresponding row was never inserted into system_params, so the service has
-- always fallen back silently to the hard-coded default (MaxMembersPerGroup = 20).
--
-- This migration makes the parameter visible and overridable by operators without
-- requiring a code change or process restart (is_runtime = TRUE).

INSERT INTO system_params (key, value, type, category, is_runtime, description) VALUES
    ('group.max_size', '20', 'int', 'group', TRUE,
     'Maximum number of active members allowed per quiniela. Enforced by the application layer on join and approval requests; requests that would exceed this limit are rejected with 409.')
ON CONFLICT (key) DO UPDATE SET
    value       = EXCLUDED.value,
    type        = EXCLUDED.type,
    category    = EXCLUDED.category,
    is_runtime  = EXCLUDED.is_runtime,
    description = EXCLUDED.description,
    updated_at  = NOW();
