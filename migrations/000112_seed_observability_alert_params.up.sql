-- Migration 000112: seed Prometheus alerting threshold params
--
-- Adds two system_params that mirror the hardcoded thresholds in
-- observability/prometheus/rules/alerting_rules.yml so operators can tune
-- alert sensitivity via the admin API without touching the rules file:
--
--   notify.outbox_lag_critical_sec  → WCQOutboxLagCritical > 300 s
--   notify.dlq_warning_threshold    → WCQDLQDepthWarning   > 10
--
-- Both are is_runtime=TRUE so changes propagate within the 30 s param cache
-- window without a worker restart.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    (
        'notify.outbox_lag_critical_sec',
        '300',
        '300',
        'int',
        'notify',
        TRUE,
        'Outbox processing lag in seconds above which Prometheus fires WCQOutboxLagCritical. Must be greater than notify.outbox_lag_alert_threshold_sec (warning = 30 s). Default: 300 s (5 minutes).'
    ),
    (
        'notify.dlq_warning_threshold',
        '10',
        '10',
        'int',
        'notify',
        TRUE,
        'Number of unresolved notification DLQ entries above which Prometheus fires WCQDLQDepthWarning. Must be less than notify.dlq_replay_alert_threshold (critical = 50). Default: 10.'
    )
ON CONFLICT (key) DO NOTHING;
