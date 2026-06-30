-- Seed auth.require_mfa system parameter.
-- When non-zero, PolicyProvider rejects tokens whose Clerk JWT does not carry
-- a completed MFA second factor (fva[1] absent or negative). Enable only after
-- configuring MFA as required in the Clerk dashboard; enabling before Clerk
-- enforces MFA will lock out all existing users.
-- is_runtime=TRUE: changes propagate within ~30 s without a process restart.
INSERT INTO system_params (key, value, type, category, is_runtime, description)
VALUES (
    'auth.require_mfa',
    '0',
    'int',
    'auth',
    TRUE,
    'Reject tokens where Clerk MFA second factor was not completed (fva[1] absent). 0=disabled, 1=enabled. Enable only after enabling MFA in the Clerk dashboard.'
)
ON CONFLICT (key) DO NOTHING;
