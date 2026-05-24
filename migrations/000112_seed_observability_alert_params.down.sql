DELETE FROM system_params WHERE key IN (
    'notify.outbox_lag_critical_sec',
    'notify.dlq_warning_threshold'
);
