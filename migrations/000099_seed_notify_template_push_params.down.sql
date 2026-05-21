DELETE FROM system_params
WHERE key IN (
    'notify.template_cache_ttl_seconds',
    'notify.push_title_max_chars',
    'notify.push_body_max_chars'
);
