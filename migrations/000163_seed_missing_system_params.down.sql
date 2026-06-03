-- Reverse of 000163: remove params that were seeded by this migration.
-- ON CONFLICT DO NOTHING in the up migration means only newly-inserted rows
-- need to be removed; rows that pre-existed are not touched here either.
DELETE FROM system_params WHERE key IN (
    -- Scoring
    'scoring.exact_score',
    'scoring.correct_outcome',
    'scoring.goal_difference',
    'scoring.penalties_bonus',
    -- Prediction
    'prediction.deadline_minutes',
    -- Group
    'group.min_members_for_active',
    'group.invite_code_length',
    -- Tournament
    'tournament.win_points',
    -- Pagination
    'pagination.default_limit',
    'pagination.max_limit',
    -- Conflict
    'conflict.stale_days',
    -- Cache
    'cache.match_ttl_seconds',
    'cache.leaderboard_ttl_seconds',
    -- API limits
    'api.body_size_limit_bytes',
    'api.idempotency_ttl_hours',
    'api.idempotency_key_max_len',
    'auth.validation_timeout_seconds',
    -- API rate limiting
    'api.rate_limit_burst',
    'admin.rate_limit_burst',
    -- IP rate limiting
    'api.ip_rate_limit_global_burst',
    'api.ip_rate_limit_webhook_rps',
    'api.ip_rate_limit_webhook_burst',
    -- Audit
    'audit.max_retries',
    'audit.retry_delay_ms',
    'audit.write_timeout_seconds',
    -- Circuit breakers
    'breaker.paypal_cert_max_fails',
    'breaker.paypal_cert_cooldown_sec',
    'breaker.file_store_max_fails',
    'breaker.file_store_cooldown_sec',
    'breaker.cache_cooldown_sec',
    -- Messaging
    'messaging.max_retries',
    'messaging.stream_max_len',
    'messaging.stream_worker_count',
    'messaging.stream_read_block_sec',
    -- Repository
    'repository.tx_retry_max_attempts',
    'repository.tx_retry_base_delay_ms',
    'repository.tx_retry_max_delay_ms',
    -- DLQ
    'dlq.sample_size',
    'dlq.replay_default_limit',
    -- FX
    'fx.sell_margin_bps',
    'fx.display_decimals',
    'fx.stale_threshold_h',
    'fx.exchange_rate_api_timeout_sec',
    'fx.open_exchange_timeout_sec',
    -- KYC
    'kyc.aml_threshold_cents',
    'kyc.ip_velocity_max_submissions',
    'kyc.max_doc_upload_bytes',
    'kyc.review_interval_days',
    'kyc.tier1_withdrawal_velocity_cents',
    'kyc.tier2_deposit_limit_cents',
    'kyc.tier2_deposit_velocity_cents',
    'kyc.tier2_payout_limit_cents',
    'kyc.tier2_withdrawal_velocity_cents',
    -- Payment
    'payment.bank_transfer_max_amount_cents',
    'payment.withdrawal_min_cents',
    'payment.withdrawal_max_cents',
    -- Notifications: outbox
    'notify.outbox_poll_interval_sec',
    'notify.outbox_lock_duration_sec',
    'notify.outbox_max_attempts',
    'notify.outbox_stale_lock_threshold_sec',
    -- Notifications: DLQ
    'notify.dlq_replay_poll_interval_sec',
    'notify.dlq_replay_max_attempts',
    'notify.dlq_replay_alert_threshold',
    'notify.dlq_warning_threshold',
    -- Notifications: push digest
    'notify.push_digest_window_sec',
    'notify.push_digest_threshold',
    -- Notifications: push content
    'notify.push_title_max_chars',
    'notify.push_body_max_chars',
    'notify.web_push_ttl_sec',
    -- Notifications: scheduling
    'notify.bank_transfer_stale_sec',
    'notify.withdrawal_stale_sec',
    'notify.high_value_withdrawal_cents',
    'notify.pending_reminder_interval_sec',
    'notify.prediction_deadline_lead_min_1',
    'notify.prediction_deadline_lead_min_2',
    'notify.prediction_missing_lead_min',
    -- Notifications: strings
    'notify.admin_emails',
    'notify.from_address',
    'notify.web_push_vapid_public_key',
    'notify.web_push_vapid_subject',
    'notify.push_icon_url',
    'notify.push_badge_url',
    -- Worker
    'worker.snapshot_concurrency',
    'worker.snapshot_retry_base_ms',
    'worker.snapshot_max_attempts',
    'worker.sched_match_result_interval_sec',
    'worker.sched_pending_reminder_interval_sec',
    'worker.sched_push_prune_interval_sec',
    'worker.sched_stale_escalation_interval_sec',
    'worker.dlq_monitor_interval_sec',
    'worker.purge_interval_hours',
    'worker.leaderboard_publish_max_attempts',
    'worker.leaderboard_publish_base_delay_ms'
);
