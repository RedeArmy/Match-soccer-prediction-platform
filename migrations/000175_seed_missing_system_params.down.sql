-- Rollback migration 000175: remove the 44 seeded rows and revert the three
-- is_runtime corrections to their pre-migration (incorrect) state.

-- ── Revert IS_RUNTIME fixes ───────────────────────────────────────────────────
UPDATE system_params SET is_runtime = TRUE  WHERE key = 'notify.dlq_replay_alert_threshold';
UPDATE system_params SET is_runtime = FALSE WHERE key = 'kyc.review_interval_days';
UPDATE system_params SET is_runtime = FALSE WHERE key = 'kyc.max_doc_upload_bytes';

-- ── Remove seeded rows ────────────────────────────────────────────────────────
DELETE FROM system_params WHERE key IN (
    'scoring.extra_time_bonus',
    'scoring.update_chunk_size',
    'group.max_size',
    'conflict.max_scan',
    'admin.bulk_max_items',
    'admin.rate_limit_rate_per_sec',
    'cache.dashboard_ttl_seconds',
    'system.purge_retention_days',
    'audit.max_in_flight',
    'system.param_history_retention_days',
    'worker.outbox_retention_days',
    'snapshot.keep_latest_count',
    'worker.sched_pred_deadline_interval_sec',
    'api.rate_limit_rate_per_sec',
    'api.ip_rate_limit_global_rps',
    'breaker.cache_max_fails',
    'payment.max_upload_bytes',
    'payment.bank_transfer_min_amount_cents',
    'payment.intent_ttl_minutes',
    'payment.intent_max_cents',
    'payment.usd_gtq_rate',
    'payment.exchange_rate_margin_bps',
    'fx.buy_margin_bps',
    'fx.history_retention_days',
    'fx.banguat_timeout_sec',
    'notify.bank_transfer_queue_depth_threshold',
    'notify.sse_heartbeat_interval_sec',
    'notify.scheduler_timezone',
    'notify.default_locale',
    'notify.template_cache_ttl_seconds',
    'notify.push_sub_retention_days',
    'notify.render_timeout_ms',
    'notify.dlq_replay_batch_size',
    'notify.outbox_batch_size',
    'notify.outbox_lag_alert_threshold_sec',
    'notify.outbox_lag_critical_sec',
    'notify.sse_chan_buf_size',
    'notify.sse_max_conns_per_user',
    'notify.sse_evict_after_drops',
    'kyc.tier1_deposit_limit_cents',
    'kyc.tier1_deposit_velocity_cents',
    'kyc.risk_dashboard_cache_ttl_sec',
    'kyc.ip_velocity_window_minutes',
    'kyc.doc_retention_years'
);
