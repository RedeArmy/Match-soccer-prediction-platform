-- Migration 000162: seed payment.intent_retention_days system parameter.
--
-- Expired pending payment intents (status='pending', expires_at elapsed)
-- accumulate indefinitely without a purge policy. Captured intents are
-- financial records and are never removed.
--
-- This parameter controls how many days AFTER expiry a pending intent is kept
-- before the daily purge job deletes it. The 7-day post-expiry window gives
-- operators time to audit any PayPal delivery anomalies without letting the
-- table grow unboundedly.
--
-- At typical load (a few hundred deposits per day for a quiniela app) the
-- table stays well under 10k rows even without purging; the purge is purely
-- a housekeeping precaution for long-running deployments.
--
-- is_runtime=FALSE: a worker restart is required to pick up a new value.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES (
    'payment.intent_retention_days',
    '7',
    '7',
    'int',
    'payment',
    FALSE,
    'Number of days expired pending payment_intents rows are kept after expiry. '
    'Only rows with status=''pending'' AND expires_at < (NOW() - retention) are '
    'deleted. Captured intents are financial records and are never purged. '
    'Default: 7 days. Valid range: 1–90. Worker restart required.'
)
ON CONFLICT (key) DO NOTHING;
