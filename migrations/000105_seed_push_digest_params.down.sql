DELETE FROM system_params
WHERE key IN (
    'notify.push_digest_window_sec',
    'notify.push_digest_threshold'
);
