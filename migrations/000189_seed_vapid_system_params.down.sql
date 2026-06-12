DELETE FROM system_params WHERE key IN (
    'notify.web_push_vapid_public_key',
    'notify.web_push_vapid_subject'
);
