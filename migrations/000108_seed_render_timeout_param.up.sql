-- Seed notify.render_timeout_ms system parameter (migration 000108).
-- Controls the wall-clock budget for a single email template render.
-- Renders that exceed this limit return an error so the outbox worker retries
-- the entry with exponential back-off rather than stalling indefinitely.
-- is_runtime=TRUE: new value takes effect within the 30 s param cache window.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.render_timeout_ms',
    '5000',
    '5000',
    'int',
    'notify',
    TRUE,
    'Maximum milliseconds allowed for a single email template render before the dispatcher returns an error and the outbox worker retries the entry.'
)
ON CONFLICT (key) DO NOTHING;
