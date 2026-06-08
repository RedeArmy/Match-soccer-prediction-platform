-- Migration 000174: update auth.validation_timeout_seconds default_value from 5 → 10.
--
-- Background: the JWKS warmup now retries up to 3 times with independent per-attempt
-- contexts. Each attempt already uses the timeout stored in this param, so 10 s per
-- attempt (30 s total budget) is the correct default. The old 5 s value was set by
-- migration 000040 and carried forward through 000163.
--
-- Only update value when it still matches the old default (5); operator overrides
-- (value != '5') are preserved.
UPDATE system_params
SET default_value = '10',
    value         = CASE WHEN value = '5' THEN '10' ELSE value END,
    updated_at    = NOW()
WHERE key = 'auth.validation_timeout_seconds';
