-- Migration: seed system parameters for the notification subsystem (Phase 0).
--
-- Eleven new parameters cover three operational areas:
--
--   notify (int)    — stale-alert thresholds, high-value withdrawal threshold,
--                     periodic-reminder interval, and prediction-deadline lead times.
--                     All are is_runtime=TRUE so changes propagate within the cache
--                     window (30 s) without a process restart.
--
--   notify (string) — admin email list and VAPID keys for Web Push.
--                     is_runtime=TRUE; updated values are picked up on the next
--                     push or email dispatch within the cache window.
--
-- Idempotent: ON CONFLICT DO NOTHING means re-running this migration is safe.

INSERT INTO system_params (key, value, default_value, type, category, is_runtime, description)
VALUES
    -- ── Stale-alert thresholds ────────────────────────────────────────────────
    (
        'notify.bank_transfer_stale_sec',
        '43200', '43200',
        'int', 'notify',
        TRUE,
        'Seconds after which an unreviewed bank-transfer proof triggers an admin stale-alert (EventAdminBankTransferStale). Default: 43200 (12 hours). Changeable at runtime.'
    ),
    (
        'notify.withdrawal_stale_sec',
        '86400', '86400',
        'int', 'notify',
        TRUE,
        'Seconds after which an unreviewed withdrawal request triggers an admin stale-alert (EventAdminWithdrawalStale). Default: 86400 (24 hours). Changeable at runtime.'
    ),

    -- ── High-value withdrawal threshold ──────────────────────────────────────
    (
        'notify.high_value_withdrawal_cents',
        '1000000', '1000000',
        'int', 'notify',
        TRUE,
        'Withdrawal amount in cents (GTQ × 100) above which EventAdminHighValueWithdrawal is emitted in addition to the regular pending alert. Default: 1000000 (Q10 000). Changeable at runtime.'
    ),

    -- ── Periodic pending-action reminder ─────────────────────────────────────
    (
        'notify.pending_reminder_interval_sec',
        '14400', '14400',
        'int', 'notify',
        TRUE,
        'Seconds between repeated admin "pending action required" alerts while a payment or withdrawal is still awaiting review. Default: 14400 (4 hours). Changeable at runtime.'
    ),

    -- ── Prediction deadline push alerts ──────────────────────────────────────
    (
        'notify.prediction_deadline_lead_min_1',
        '60', '60',
        'int', 'notify',
        TRUE,
        'First (earlier) lead time in minutes before prediction deadline closes at which a push alert is sent. Default: 60 minutes. Changeable at runtime.'
    ),
    (
        'notify.prediction_deadline_lead_min_2',
        '15', '15',
        'int', 'notify',
        TRUE,
        'Second (later, closer) lead time in minutes before prediction deadline closes at which a push alert is sent. Default: 15 minutes. Changeable at runtime.'
    ),
    (
        'notify.prediction_missing_lead_min',
        '120', '120',
        'int', 'notify',
        TRUE,
        'Lead time in minutes before kick-off at which a missing-prediction reminder push alert is sent. Default: 120 minutes (2 hours). Changeable at runtime.'
    ),

    -- ── Admin email list ──────────────────────────────────────────────────────
    (
        'notify.admin_emails',
        '', '',
        'string', 'notify',
        TRUE,
        'Comma-separated list of email addresses that receive all admin.* and system.* notification events. An empty value disables admin email delivery. Changeable at runtime.'
    ),

    -- ── VAPID keys for Web Push ───────────────────────────────────────────────
    -- Note: the private key is intentionally absent — it must be injected via
    -- WCQ_WEBPUSH_VAPIDPRIVATEKEY and must never be stored in system_params.
    (
        'notify.web_push_vapid_public_key',
        '', '',
        'string', 'notify',
        TRUE,
        'VAPID public key (base64url-encoded P-256 point, RFC 8292) used to authenticate Web Push subscription requests. An empty value disables Web Push delivery. Changeable at runtime.'
    ),
    (
        'notify.web_push_vapid_subject',
        '', '',
        'string', 'notify',
        TRUE,
        'VAPID subject claim — an HTTPS URL or mailto: address that identifies this application server to push services (RFC 8292 §2.1). An empty value disables Web Push delivery. Changeable at runtime.'
    )
ON CONFLICT (key) DO NOTHING;
