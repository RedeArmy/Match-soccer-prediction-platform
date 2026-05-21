-- Migration 000099: seed system parameters for notification template cache TTL
-- and push notification payload limits.
--
-- notify.template_cache_ttl_seconds  — how long the in-memory template cache is
--   considered fresh before the next read re-fetches from the DB.
-- notify.push_title_max_chars        — maximum runes kept in the push title field
--   (Android FCM hard limit is 100; iOS allows up to 250).
-- notify.push_body_max_chars         — maximum runes kept in the push body field
--   (Android FCM hard limit is 300).

INSERT INTO system_params (key, value, type, category, is_runtime, description)
VALUES
    ('notify.template_cache_ttl_seconds', '300',  'int', 'notify', TRUE,
     'TTL in seconds for the in-memory notification template cache (30–3600).'),
    ('notify.push_title_max_chars',       '100',  'int', 'notify', TRUE,
     'Maximum rune count for push notification title field (10–500).'),
    ('notify.push_body_max_chars',        '300',  'int', 'notify', TRUE,
     'Maximum rune count for push notification body field (50–2000).')
ON CONFLICT (key) DO NOTHING;
