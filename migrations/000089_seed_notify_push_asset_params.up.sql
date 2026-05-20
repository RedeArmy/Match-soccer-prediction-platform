-- Seed system_params for Web Push notification asset URLs (Phase 3).
-- Both parameters are is_runtime=TRUE; changes propagate within the 30 s cache
-- window without a process restart, allowing CDN or branding updates without a
-- deploy.  Idempotent: ON CONFLICT DO NOTHING.
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.push_icon_url',
        '/icons/icon-192.png', '/icons/icon-192.png',
        'string', 'notify',
        TRUE,
        'URL of the 192×192 px PNG notification icon shown by the browser for Web Push messages. Override to use a CDN path or branding-specific asset. Default: /icons/icon-192.png. Changeable at runtime.'
    ),
    (
        'notify.push_badge_url',
        '/icons/badge-72.png', '/icons/badge-72.png',
        'string', 'notify',
        TRUE,
        'URL of the 72×72 px monochrome PNG badge icon displayed in the Android notification bar. Override to use a CDN path or branding-specific asset. Default: /icons/badge-72.png. Changeable at runtime.'
    )
ON CONFLICT (key) DO NOTHING;
