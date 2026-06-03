-- Migration 000163: seed all system_params rows that were missing from the table.
--
-- AllParamKeys() in internal/domain/constants.go defines 138 parameters, but only
-- 48 had corresponding INSERT migrations. This migration seeds the remaining 90
-- parameters so every knob is visible and overridable via the admin API without
-- requiring a code change. Parameters with is_runtime=FALSE still require a process
-- restart to take effect, but they will at least appear in the admin panel.
--
-- ON CONFLICT DO NOTHING: safe to re-run; rows already present are left untouched.
-- Operators who have already tuned a value via direct SQL keep their override.
--
-- Grouped by category for readability.

-- ── Scoring ──────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('scoring.exact_score',      '5', '5', 'int', 'scoring', TRUE,
     'Points awarded for a prediction with the exact home and away score. Runtime: yes.'),
    ('scoring.correct_outcome',  '2', '2', 'int', 'scoring', TRUE,
     'Points awarded for predicting the correct match result (win/draw/loss) without the exact score. Runtime: yes.'),
    ('scoring.goal_difference',  '1', '1', 'int', 'scoring', TRUE,
     'Bonus points awarded when the predicted goal difference matches the actual result. Runtime: yes.'),
    ('scoring.penalties_bonus',  '0', '0', 'int', 'scoring', TRUE,
     'Bonus points for correctly predicting a penalty-shootout win method. Default 0 (disabled). Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Prediction ───────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('prediction.deadline_minutes', '5', '5', 'int', 'prediction', TRUE,
     'Minutes before kickoff after which predictions are no longer accepted. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Group ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('group.min_members_for_active', '5',  '5',  'int', 'group', TRUE,
     'Minimum number of active paid members required before a quiniela is eligible for payment and prize distribution. Runtime: yes.'),
    ('group.invite_code_length',     '10', '10', 'int', 'group', TRUE,
     'Number of characters in generated invite codes. Longer codes are harder to guess; shorter codes are easier to share. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Tournament ───────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('tournament.win_points', '3', '3', 'int', 'tournament', TRUE,
     'Standing points awarded for a group-stage win (FIFA 3-point rule). Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Pagination ───────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('pagination.default_limit', '50',  '50',  'int', 'pagination', TRUE,
     'Default page size returned by list endpoints when the client omits ?limit. Runtime: yes.'),
    ('pagination.max_limit',     '200', '200', 'int', 'pagination', TRUE,
     'Maximum page size a client may request. Requests exceeding this value are clamped. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Conflict ─────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('conflict.stale_days', '7', '7', 'int', 'conflict', TRUE,
     'Age in days after which a pending payment record is considered stale and surfaced by ConflictService. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Cache ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('cache.match_ttl_seconds',       '300', '300', 'int', 'cache', FALSE,
     'TTL in seconds for the match-list Redis cache. Lower values increase DB load; higher values delay visibility of score updates. Restart required.'),
    ('cache.leaderboard_ttl_seconds', '60',  '60',  'int', 'cache', TRUE,
     'TTL in seconds for the leaderboard Redis cache. Mutation of this param flushes and repopulates the cache immediately. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── API: request limits ───────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('api.body_size_limit_bytes',   '65536', '65536', 'int', 'api', FALSE,
     'Maximum request body size in bytes for non-upload endpoints. Requests exceeding this are rejected with 413. Restart required.'),
    ('api.idempotency_ttl_hours',   '24',    '24',    'int', 'api', FALSE,
     'Hours a committed idempotency entry is retained. Within this window, replaying the same Idempotency-Key replays the cached response. Restart required.'),
    ('api.idempotency_key_max_len', '255',   '255',   'int', 'api', FALSE,
     'Maximum byte length of a client-supplied Idempotency-Key header value. Keys exceeding this are rejected with 422. Restart required.'),
    ('auth.validation_timeout_seconds', '5', '5',     'int', 'system', FALSE,
     'Timeout in seconds for the JWKS warmup fetch at server startup. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── API: per-user rate limiting ───────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('api.rate_limit_burst', '30', '30', 'int', 'api', FALSE,
     'Maximum burst size of the per-user token bucket on /api/v1 endpoints. LimiterStore is constructed once at startup; restart required to apply.'),
    ('admin.rate_limit_burst', '10', '10', 'int', 'admin', FALSE,
     'Maximum burst size of the per-admin token bucket on /api/v1/admin endpoints. In-process only; mutation hook applies without restart (admin.rate_limit_rate_per_sec too).')
ON CONFLICT (key) DO NOTHING;

-- ── API: IP-based rate limiting ───────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('api.ip_rate_limit_global_burst',  '100', '100', 'int', 'api', FALSE,
     'Maximum burst size of the L1 per-IP token bucket applied to all routes. Restart required (Redis-backed) or mutation applies immediately (in-process fallback).'),
    ('api.ip_rate_limit_webhook_rps',   '5',   '5',   'int', 'api', FALSE,
     'Per-IP refill rate (tokens/second) for the L2 stricter bucket applied only to /webhooks/* routes. Restart required.'),
    ('api.ip_rate_limit_webhook_burst', '10',  '10',  'int', 'api', FALSE,
     'Maximum burst size of the L2 per-IP webhook bucket. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Audit ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('audit.max_retries',        '2',   '2',   'int', 'system', FALSE,
     'Number of write attempts before an audit log entry is permanently lost. Restart required.'),
    ('audit.retry_delay_ms',     '250', '250', 'int', 'system', FALSE,
     'Delay in milliseconds between audit write retry attempts. Restart required.'),
    ('audit.write_timeout_seconds', '5', '5',  'int', 'system', FALSE,
     'Timeout in seconds for a single audit log DB write. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Circuit breakers ─────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('breaker.paypal_cert_max_fails',    '3',  '3',  'int', 'breaker', FALSE,
     'Consecutive PayPal cert-fetch failures before the circuit opens. While open, PayPal webhooks return 500. Restart required.'),
    ('breaker.paypal_cert_cooldown_sec', '60', '60', 'int', 'breaker', FALSE,
     'Seconds the PayPal cert circuit stays open before allowing a single trial request. Restart required.'),
    ('breaker.file_store_max_fails',     '5',  '5',  'int', 'breaker', FALSE,
     'Consecutive storage errors (S3/GDrive/OneDrive) before the file-store circuit opens. Restart required.'),
    ('breaker.file_store_cooldown_sec',  '30', '30', 'int', 'breaker', FALSE,
     'Seconds the file-store circuit stays open before allowing a trial request. Restart required.'),
    ('breaker.cache_cooldown_sec',       '30', '30', 'int', 'breaker', FALSE,
     'Seconds the Redis cache circuit stays open before allowing a trial request. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Messaging (Event Bus) ────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('messaging.max_retries',          '3',      '3',      'int', 'messaging', FALSE,
     'Maximum retry attempts for event delivery before an event is moved to the DLQ. Restart required.'),
    ('messaging.stream_max_len',       '600000', '600000', 'int', 'messaging', FALSE,
     'Maximum length of each Redis Stream (MAXLEN ~ trimming). Controls memory usage. Restart required.'),
    ('messaging.stream_worker_count',  '8',      '8',      'int', 'messaging', FALSE,
     'Number of goroutines in the per-EventType worker pool that processes stream messages. Restart required.'),
    ('messaging.stream_read_block_sec','5',       '5',      'int', 'messaging', FALSE,
     'XREADGROUP block timeout in seconds. Smaller values react faster to shutdown signals at the cost of more idle round-trips. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Repository TX retry policy ────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('repository.tx_retry_max_attempts', '3',    '3',    'int', 'repository', FALSE,
     'Total transaction attempts (including first) before a transient serialization error is returned. Restart required.'),
    ('repository.tx_retry_base_delay_ms','50',   '50',   'int', 'repository', FALSE,
     'Base backoff delay in milliseconds between retry attempts (equal-jitter). Restart required.'),
    ('repository.tx_retry_max_delay_ms', '1000', '1000', 'int', 'repository', FALSE,
     'Maximum backoff cap in milliseconds. Prevents unreasonably long waits at high retry counts. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── DLQ monitoring ───────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('dlq.sample_size',           '5',  '5',  'int', 'dlq', FALSE,
     'Number of DLQ entries sampled per monitor tick for log output. Restart required.'),
    ('dlq.replay_default_limit',  '10', '10', 'int', 'dlq', FALSE,
     'Default page size for admin DLQ replay endpoints when no limit is specified. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── FX: missing params ────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('fx.sell_margin_bps',                 '200', '200', 'int', 'fx', TRUE,
     'Sell-rate markup in basis points applied to the raw USD/GTQ rate. 200 bps = +2%. Runtime: yes.'),
    ('fx.display_decimals',                '4',   '4',   'int', 'fx', TRUE,
     'Number of decimal places shown in exchange rate display responses. Runtime: yes.'),
    ('fx.stale_threshold_h',               '26',  '26',  'int', 'fx', TRUE,
     'Hours after which an exchange rate is considered stale and the admin panel shows a warning. Runtime: yes.'),
    ('fx.exchange_rate_api_timeout_sec',   '5',   '5',   'int', 'fx', FALSE,
     'HTTP timeout in seconds for calls to v6.exchangerate-api.com (secondary fallback). Restart required.'),
    ('fx.open_exchange_timeout_sec',       '5',   '5',   'int', 'fx', FALSE,
     'HTTP timeout in seconds for calls to openexchangerates.org (tertiary fallback). Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── KYC: missing params ───────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('kyc.aml_threshold_cents',             '2500000',  '2500000',  'int', 'kyc', TRUE,
     'Transaction amount in cents (GTQ) above which an AML flag is logged. Default Q25,000. Runtime: yes.'),
    ('kyc.ip_velocity_max_submissions',     '3',        '3',        'int', 'kyc', TRUE,
     'Maximum KYC submissions allowed per IP within the ip_velocity_window_minutes window. Runtime: yes.'),
    ('kyc.max_doc_upload_bytes',            '10485760', '10485760', 'int', 'kyc', FALSE,
     'Maximum KYC document upload size in bytes (10 MB default). Restart required.'),
    ('kyc.review_interval_days',            '365',      '365',      'int', 'kyc', FALSE,
     'Days between mandatory re-review cycles for approved KYC profiles. Restart required.'),
    ('kyc.tier1_withdrawal_velocity_cents', '0',        '0',        'int', 'kyc', TRUE,
     'Rolling 24-hour withdrawal velocity cap in cents for Tier 1 users. 0 = no limit. Runtime: yes.'),
    ('kyc.tier2_deposit_limit_cents',       '1500000',  '1500000',  'int', 'kyc', TRUE,
     'Single-transaction deposit limit in cents for Tier 2 users. Default Q15,000. Runtime: yes.'),
    ('kyc.tier2_deposit_velocity_cents',    '20000000', '20000000', 'int', 'kyc', TRUE,
     'Rolling 24-hour deposit velocity cap in cents for Tier 2 users. Default Q200,000. Runtime: yes.'),
    ('kyc.tier2_payout_limit_cents',        '1500000',  '1500000',  'int', 'kyc', TRUE,
     'Single-transaction payout limit in cents for Tier 2 users. Default Q15,000. Runtime: yes.'),
    ('kyc.tier2_withdrawal_velocity_cents', '10000000', '10000000', 'int', 'kyc', TRUE,
     'Rolling 24-hour withdrawal velocity cap in cents for Tier 2 users. Default Q100,000. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Payment: missing params ───────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('payment.bank_transfer_max_amount_cents', '10000000', '10000000', 'int', 'payment', TRUE,
     'Maximum declared amount in cents for a bank transfer proof upload. Default Q100,000. Runtime: yes.'),
    ('payment.withdrawal_min_cents',           '5000',     '5000',     'int', 'payment', TRUE,
     'Minimum withdrawal amount in cents. Default Q50. Runtime: yes.'),
    ('payment.withdrawal_max_cents',           '500000',   '500000',   'int', 'payment', TRUE,
     'Maximum withdrawal amount in cents. Default Q5,000. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: outbox worker ─────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.outbox_poll_interval_sec',       '2',   '2',   'int', 'notify', FALSE,
     'Seconds between outbox poll cycles. Lower values reduce delivery latency at the cost of more idle DB round-trips. Restart required.'),
    ('notify.outbox_lock_duration_sec',       '300', '300', 'int', 'notify', FALSE,
     'Seconds a claimed outbox row is locked before the stale-lock recovery job reclaims it. Must exceed the longest expected dispatch. Restart required.'),
    ('notify.outbox_max_attempts',            '5',   '5',   'int', 'notify', FALSE,
     'Maximum dispatch attempts before an outbox entry is moved to the DLQ. Restart required.'),
    ('notify.outbox_stale_lock_threshold_sec','600', '600', 'int', 'notify', FALSE,
     'Seconds after which a locked-but-unprocessed outbox row is considered stale and eligible for reclaim. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: DLQ replay ────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.dlq_replay_poll_interval_sec', '30', '30', 'int', 'notify', FALSE,
     'Seconds between DLQ replay poll cycles. Restart required.'),
    ('notify.dlq_replay_max_attempts',      '5',  '5',  'int', 'notify', FALSE,
     'Maximum DLQ replay attempts before an entry is permanently unresolved. Restart required.'),
    ('notify.dlq_replay_alert_threshold',   '50', '50', 'int', 'notify', TRUE,
     'Unresolved DLQ entry count that triggers a critical alert. Runtime: yes.'),
    ('notify.dlq_warning_threshold',        '10', '10', 'int', 'notify', TRUE,
     'Unresolved DLQ entry count that triggers a warning alert. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: push digest ────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.push_digest_window_sec',  '300', '300', 'int', 'notify', FALSE,
     'Window in seconds within which multiple push notifications are collapsed into one digest. Restart required.'),
    ('notify.push_digest_threshold',   '5',   '5',   'int', 'notify', FALSE,
     'Number of messages within the digest window that triggers collapsing into a digest push. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: push content limits ───────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.push_title_max_chars', '100', '100', 'int', 'notify', TRUE,
     'Maximum characters in a push notification title. Longer titles are truncated. Runtime: yes.'),
    ('notify.push_body_max_chars',  '300', '300', 'int', 'notify', TRUE,
     'Maximum characters in a push notification body. Longer bodies are truncated. Runtime: yes.'),
    ('notify.web_push_ttl_sec',     '86400', '86400', 'int', 'notify', TRUE,
     'TTL in seconds for Web Push messages. Notifications not delivered within this window are dropped. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: scheduling intervals ──────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.bank_transfer_stale_sec',            '43200', '43200', 'int', 'notify', TRUE,
     'Age in seconds after which an unreviewed bank transfer proof triggers a stale-review notification. Default 12 h. Runtime: yes.'),
    ('notify.withdrawal_stale_sec',               '86400', '86400', 'int', 'notify', TRUE,
     'Age in seconds after which an unprocessed withdrawal triggers a stale notification. Default 24 h. Runtime: yes.'),
    ('notify.high_value_withdrawal_cents',        '1000000', '1000000', 'int', 'notify', TRUE,
     'Withdrawal amount in cents above which an admin alert is fired. Default Q10,000. Runtime: yes.'),
    ('notify.pending_reminder_interval_sec',      '14400', '14400', 'int', 'notify', TRUE,
     'Interval in seconds between bank-transfer and withdrawal reminder notifications. Default 4 h. Runtime: yes.'),
    ('notify.prediction_deadline_lead_min_1',     '60',  '60',  'int', 'notify', TRUE,
     'First reminder before prediction deadline, in minutes before kickoff. Runtime: yes.'),
    ('notify.prediction_deadline_lead_min_2',     '15',  '15',  'int', 'notify', TRUE,
     'Second (final) reminder before prediction deadline, in minutes before kickoff. Runtime: yes.'),
    ('notify.prediction_missing_lead_min',        '120', '120', 'int', 'notify', TRUE,
     'Minutes before kickoff to send a missing-prediction reminder to members who have not yet submitted. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications: string params ─────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.admin_emails',            '',                    '',                    'string', 'notify', TRUE,
     'Comma-separated list of admin email addresses that receive operational alerts. Empty = alerts disabled. Runtime: yes.'),
    ('notify.from_address',            '',                    '',                    'string', 'notify', TRUE,
     'Sender address for outgoing transactional emails (e.g. "Quiniela <noreply@domain.gt>"). Runtime: yes.'),
    ('notify.web_push_vapid_public_key','',                   '',                    'string', 'notify', TRUE,
     'VAPID public key for Web Push subscriptions (Base64url-encoded). Required for push notifications. Runtime: yes.'),
    ('notify.web_push_vapid_subject',  '',                    '',                    'string', 'notify', TRUE,
     'VAPID subject URI (mailto: or https:) identifying the push service operator. Runtime: yes.'),
    ('notify.push_icon_url',           '/icons/icon-192.png', '/icons/icon-192.png', 'string', 'notify', TRUE,
     'URL of the icon shown in push notifications. Must be accessible from the browser. Runtime: yes.'),
    ('notify.push_badge_url',          '/icons/badge-72.png', '/icons/badge-72.png', 'string', 'notify', TRUE,
     'URL of the monochrome badge icon shown in push notifications on Android. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Worker: snapshot generation ──────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('worker.snapshot_concurrency',    '4',   '4',   'int', 'worker', FALSE,
     'Maximum concurrent quiniela snapshot writes per MatchFinished event. Restart required.'),
    ('worker.snapshot_retry_base_ms',  '100', '100', 'int', 'worker', FALSE,
     'Initial snapshot write retry backoff in milliseconds; doubles on each subsequent attempt. Restart required.'),
    ('worker.snapshot_max_attempts',   '3',   '3',   'int', 'worker', FALSE,
     'Maximum snapshot write attempts per quiniela per match event before the error is logged. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Worker: scheduler intervals ──────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('worker.sched_match_result_interval_sec',     '900',   '900',   'int', 'worker', FALSE,
     'Scheduler tick interval in seconds for the match-result reminder job. Default 15 min. Restart required.'),
    ('worker.sched_pending_reminder_interval_sec', '14400', '14400', 'int', 'worker', FALSE,
     'Scheduler tick interval in seconds for the pending-transfer/withdrawal reminder job. Default 4 h. Restart required.'),
    ('worker.sched_push_prune_interval_sec',       '86400', '86400', 'int', 'worker', FALSE,
     'Scheduler tick interval in seconds for the push-subscription pruning job. Default 24 h. Restart required.'),
    ('worker.sched_stale_escalation_interval_sec', '1800',  '1800',  'int', 'worker', FALSE,
     'Scheduler tick interval in seconds for the stale-payment escalation job. Default 30 min. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Worker: DLQ and purge intervals ──────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('worker.dlq_monitor_interval_sec',           '300', '300', 'int', 'worker', FALSE,
     'Seconds between DLQ depth monitor ticks. Restart required.'),
    ('worker.purge_interval_hours',               '24',  '24',  'int', 'worker', FALSE,
     'Hours between purge daemon ticks. Default daily. Restart required.'),
    ('worker.leaderboard_publish_max_attempts',   '3',   '3',   'int', 'worker', FALSE,
     'Maximum leaderboard-signal retry attempts per quiniela after scoring. Restart required.'),
    ('worker.leaderboard_publish_base_delay_ms',  '50',  '50',  'int', 'worker', FALSE,
     'Base delay in milliseconds for leaderboard-signal exponential backoff. Restart required.')
ON CONFLICT (key) DO NOTHING;
