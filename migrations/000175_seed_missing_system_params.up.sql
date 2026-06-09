-- Migration 000175: seed system_params rows added after migration 000163 and fix
-- three is_runtime mismatches introduced by earlier migrations.
--
-- 44 MISSING rows: parameters added by features between migration 000163 and the
-- current codebase that were never seeded into the database.
-- 3 IS_RUNTIME fixes: rows that were seeded with the wrong is_runtime flag.
--
-- ON CONFLICT DO NOTHING: safe to re-run; rows already present are untouched.
-- Operators who have already tuned a value via the admin API keep their override.
--
-- Grouped by category for readability.

-- ── Scoring ──────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('scoring.extra_time_bonus', '0', '0', 'int', 'scoring', TRUE,
     'Bonus points for correctly predicting a match that goes to extra time. Default 0 (disabled). Runtime: yes.'),
    ('scoring.update_chunk_size', '500', '500', 'int', 'scoring', FALSE,
     'Maximum number of predictions updated in a single scoring batch. Restart required to change.')
ON CONFLICT (key) DO NOTHING;

-- ── Group ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('group.max_size', '20', '20', 'int', 'group', TRUE,
     'Maximum number of active members allowed per quiniela group. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Conflict ─────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('conflict.max_scan', '5000', '5000', 'int', 'conflict', TRUE,
     'Maximum number of conflict records loaded into memory during a single scan pass. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── Admin ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('admin.bulk_max_items', '1000', '1000', 'int', 'admin', TRUE,
     'Maximum number of items that may be processed in a single admin bulk operation. Runtime: yes.'),
    ('admin.rate_limit_rate_per_sec', '2', '2', 'int', 'admin', FALSE,
     'Token-bucket refill rate (tokens/second) for the /api/v1/admin subrouter. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Cache ────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('cache.dashboard_ttl_seconds', '30', '30', 'int', 'cache', TRUE,
     'TTL in seconds for cached admin dashboard reads. Propagates within the cache window. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── System ───────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('system.purge_retention_days', '30', '30', 'int', 'system', FALSE,
     'Number of days after which soft-deleted records are eligible for hard purge. Restart required.'),
    ('audit.max_in_flight', '1000', '1000', 'int', 'system', FALSE,
     'Maximum number of concurrent in-flight audit-log writes before backpressure is applied. Restart required.'),
    ('system.param_history_retention_days', '90', '90', 'int', 'system', FALSE,
     'Number of days parameter mutation history records are retained before purge. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Worker ───────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('worker.outbox_retention_days', '30', '30', 'int', 'worker', FALSE,
     'Number of days terminal-state outbox rows (done/failed) are kept before purge. Restart required.'),
    ('snapshot.keep_latest_count', '5', '5', 'int', 'worker', FALSE,
     'Number of the most recent leaderboard snapshots to retain per group. Restart required.'),
    ('worker.sched_pred_deadline_interval_sec', '300', '300', 'int', 'worker', FALSE,
     'Interval in seconds between prediction-deadline notification scheduler runs. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── API / Rate limiting ───────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('api.rate_limit_rate_per_sec', '10', '10', 'int', 'api', FALSE,
     'Token-bucket refill rate (tokens/second) applied per authenticated user on /api/v1 routes. Restart required.'),
    ('api.ip_rate_limit_global_rps', '50', '50', 'int', 'api', FALSE,
     'L1 global IP rate limit: maximum requests per second from a single source IP across all /api/v1 routes. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Breaker ───────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('breaker.cache_max_fails', '5', '5', 'int', 'breaker', FALSE,
     'Number of consecutive Redis cache failures before the circuit breaker opens. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Payment ───────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('payment.max_upload_bytes', '5242880', '5242880', 'int', 'payment', TRUE,
     'Maximum file size in bytes for bank-transfer proof uploads. Runtime: yes.'),
    ('payment.bank_transfer_min_amount_cents', '1000', '1000', 'int', 'payment', TRUE,
     'Minimum deposit amount in GTQ centavos for bank-transfer deposits (1000 = Q10). Runtime: yes.'),
    ('payment.intent_ttl_minutes', '60', '60', 'int', 'payment', TRUE,
     'Minutes before a pending PayPal payment intent expires and is voided. Runtime: yes.'),
    ('payment.intent_max_cents', '50000', '50000', 'int', 'payment', TRUE,
     'Maximum payment intent amount in GTQ centavos (50000 = Q500). Runtime: yes.'),
    ('payment.usd_gtq_rate', '790', '790', 'int', 'payment', TRUE,
     'GTQ centavos per USD dollar used as fallback when live exchange rate is unavailable (790 = Q7.90). Runtime: yes.'),
    ('payment.exchange_rate_margin_bps', '200', '200', 'int', 'payment', TRUE,
     'Safety markup in basis points applied to the exchange rate before reserving GTQ funds. Runtime: yes.')
ON CONFLICT (key) DO NOTHING;

-- ── FX ────────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('fx.buy_margin_bps', '150', '150', 'int', 'fx', TRUE,
     'Platform buy-side margin in basis points applied on top of the mid-market rate. Runtime: yes.'),
    ('fx.history_retention_days', '90', '90', 'int', 'fx', FALSE,
     'Number of days exchange_rate_history rows are kept before purge. Restart required.'),
    ('fx.banguat_timeout_sec', '10', '10', 'int', 'fx', FALSE,
     'HTTP request timeout in seconds for the Banguat XML feed. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── Notifications ─────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('notify.bank_transfer_queue_depth_threshold', '20', '20', 'int', 'notify', TRUE,
     'Pending bank-transfer queue depth above which an alert notification is sent. Runtime: yes.'),
    ('notify.sse_heartbeat_interval_sec', '30', '30', 'int', 'notify', TRUE,
     'Interval in seconds between SSE keep-alive heartbeat comments. Runtime: yes.'),
    ('notify.scheduler_timezone', 'America/Guatemala', 'America/Guatemala', 'string', 'notify', FALSE,
     'IANA timezone used by the notification scheduler for cron-style jobs. Restart required.'),
    ('notify.default_locale', 'es', 'es', 'string', 'notify', TRUE,
     'Default BCP-47 locale for user-facing notification text when no user preference is set. Runtime: yes.'),
    ('notify.template_cache_ttl_seconds', '300', '300', 'int', 'notify', TRUE,
     'Seconds the notification template renderer caches compiled templates. Runtime: yes.'),
    ('notify.push_sub_retention_days', '30', '30', 'int', 'notify', TRUE,
     'Days after inactivation before a web-push subscription is pruned. Runtime: yes.'),
    ('notify.render_timeout_ms', '5000', '5000', 'int', 'notify', TRUE,
     'Wall-clock budget in milliseconds for a single notification template render. Runtime: yes.'),
    ('notify.dlq_replay_batch_size', '20', '20', 'int', 'notify', FALSE,
     'Maximum number of notification_dlq rows processed per replay worker batch. Restart required.'),
    ('notify.outbox_batch_size', '50', '50', 'int', 'notify', FALSE,
     'Maximum number of domain_outbox rows claimed per dispatch worker iteration. Restart required.'),
    ('notify.outbox_lag_alert_threshold_sec', '30', '30', 'int', 'notify', FALSE,
     'Outbox lag in seconds above which a warning alert fires. Restart required.'),
    ('notify.outbox_lag_critical_sec', '300', '300', 'int', 'notify', TRUE,
     'Outbox lag in seconds above which a critical alert fires (must exceed outbox_lag_alert_threshold_sec). Runtime: yes.'),
    ('notify.sse_chan_buf_size', '64', '64', 'int', 'notify', FALSE,
     'Capacity of the buffered channel per SSE connection. Restart required.'),
    ('notify.sse_max_conns_per_user', '5', '5', 'int', 'notify', FALSE,
     'Maximum concurrent SSE connections per user (0 = unlimited). Restart required.'),
    ('notify.sse_evict_after_drops', '5', '5', 'int', 'notify', FALSE,
     'Number of consecutive failed SSE sends before a connection is evicted. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── KYC ───────────────────────────────────────────────────────────────────────
INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description) VALUES
    ('kyc.tier1_deposit_limit_cents', '250000', '250000', 'int', 'kyc', TRUE,
     'Cumulative deposit limit in GTQ centavos for Tier 0/1 users (250000 = Q2,500). Runtime: yes.'),
    ('kyc.tier1_deposit_velocity_cents', '5000000', '5000000', 'int', 'kyc', TRUE,
     'Rolling 24-hour deposit velocity cap in GTQ centavos for Tier 0/1 users (5000000 = Q50,000). Runtime: yes.'),
    ('kyc.risk_dashboard_cache_ttl_sec', '60', '60', 'int', 'kyc', TRUE,
     'TTL in seconds for the KYC risk dashboard cache. Runtime: yes.'),
    ('kyc.ip_velocity_window_minutes', '60', '60', 'int', 'kyc', TRUE,
     'Rolling window in minutes for IP-based KYC submission velocity checks. Runtime: yes.'),
    ('kyc.doc_retention_years', '5', '5', 'int', 'kyc', FALSE,
     'Number of years KYC documents are retained before eligible for deletion. Restart required.')
ON CONFLICT (key) DO NOTHING;

-- ── IS_RUNTIME fixes ──────────────────────────────────────────────────────────
-- notify.dlq_replay_alert_threshold was seeded with is_runtime=TRUE but the
-- allParams spec declares it FALSE (restart required; not a hot-patched knob).
UPDATE system_params
SET is_runtime = FALSE
WHERE key = 'notify.dlq_replay_alert_threshold'
  AND is_runtime = TRUE;

-- kyc.review_interval_days was seeded with is_runtime=FALSE but the allParams
-- spec declares it TRUE (enforced per-request within the 30 s cache window).
UPDATE system_params
SET is_runtime = TRUE
WHERE key = 'kyc.review_interval_days'
  AND is_runtime = FALSE;

-- kyc.max_doc_upload_bytes was seeded with is_runtime=FALSE but the allParams
-- spec declares it TRUE (enforced per-request within the 30 s cache window).
UPDATE system_params
SET is_runtime = TRUE
WHERE key = 'kyc.max_doc_upload_bytes'
  AND is_runtime = FALSE;
