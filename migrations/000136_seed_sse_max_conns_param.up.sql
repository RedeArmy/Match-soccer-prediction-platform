-- Migration: seed notify.sse_max_conns_per_user
--
-- Adds the per-user SSE connection cap to system_params. When a user's open
-- connection count reaches this value, hub.Connect returns nil and the SSE
-- handler responds HTTP 429. A value of 0 disables the cap entirely.
--
-- is_runtime=FALSE: the hub reads this once at construction time (server startup
-- via Routes()); changing it requires an API server restart.
--
-- 5 connections covers a user with the app open in multiple browser tabs and a
-- mobile device simultaneously, while preventing runaway goroutine/heap growth
-- from a single user opening hundreds of connections.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'notify.sse_max_conns_per_user',
    '5',
    '5',
    'int',
    'notify',
    FALSE,
    'Maximum concurrent SSE connections per authenticated user. hub.Connect returns nil when reached; handler responds HTTP 429. 0 = unlimited. Requires API restart to take effect.'
)
ON CONFLICT (key) DO NOTHING;
