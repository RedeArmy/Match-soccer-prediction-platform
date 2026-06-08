-- Revert auth.validation_timeout_seconds default_value to 5.
-- value is reset only when it currently matches the new default (10).
UPDATE system_params
SET default_value = '5',
    value         = CASE WHEN value = '10' THEN '5' ELSE value END,
    updated_at    = NOW()
WHERE key = 'auth.validation_timeout_seconds';
