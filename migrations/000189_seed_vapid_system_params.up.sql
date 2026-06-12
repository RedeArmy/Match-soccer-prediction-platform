-- Migration 000189: seed VAPID web-push system parameters.
--
-- These rows are seeded with placeholder values. Update them with the real
-- VAPID key pair before enabling web-push notifications in production:
--
--   UPDATE system_params SET value = '<base64url-public-key>'
--     WHERE key = 'notify.web_push_vapid_public_key';
--
--   UPDATE system_params SET value = 'mailto:admin@yourdomain.com'
--     WHERE key = 'notify.web_push_vapid_subject';
--
-- Generate a key pair with: npx web-push generate-vapid-keys
-- The matching private key goes in WCQ_WEBPUSH_VAPIDPRIVATEKEY (server env var).
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.web_push_vapid_public_key',
     'CHANGE_ME', 'CHANGE_ME', 'string', 'notify', true,
     'VAPID public key (base64url-encoded, uncompressed P-256 point). Generate with: npx web-push generate-vapid-keys. Must match WCQ_WEBPUSH_VAPIDPRIVATEKEY.'),
    ('notify.web_push_vapid_subject',
     'mailto:admin@example.com', 'mailto:admin@example.com', 'string', 'notify', true,
     'VAPID subject claim — must be an HTTPS URL or mailto: address identifying the application operator.')
ON CONFLICT (key) DO NOTHING;
