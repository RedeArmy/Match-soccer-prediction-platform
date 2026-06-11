-- Migration 000187: system clock override parameter for development and test environments.
--
-- system.date accepts an RFC 3339 datetime string (e.g. '2026-06-20T15:00:00Z').
-- An empty value (the default) means "use real wall-clock time".
--
-- DEV/TEST ONLY — the application ignores this parameter in production mode
-- (WCQ_ENVIRONMENT != 'dev' / 'development' / 'test'). Setting a non-empty
-- value in production has no effect and should be avoided.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('system.date',
     '', '', 'string', 'system', true,
     '[DEV/TEST ONLY] Overrides the application wall clock. Accepts an RFC 3339 datetime (e.g. 2026-06-20T15:00:00Z). Empty = use real time. Ignored in production.')
ON CONFLICT (key) DO NOTHING;
