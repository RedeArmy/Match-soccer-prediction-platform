DELETE FROM system_params
WHERE key IN (
    'notify.push_icon_url',
    'notify.push_badge_url'
);
