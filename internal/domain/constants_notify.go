package domain

// PushDefaultIcon is the URL of the notification icon shown by the browser
// when no event-specific icon is provided. The asset must be served by the
// application at this path (192×192 px, PNG).
const PushDefaultIcon = "/icons/icon-192.png"

// PushDefaultBadge is the URL of the monochrome badge icon displayed in the
// Android notification bar (72×72 px, PNG).
const PushDefaultBadge = "/icons/badge-72.png"

// Default values for notification system parameters.
const (
	// Stale-alert timers: outbox-worker flags a pending operation as stale
	// once it has waited longer than these thresholds without an admin action.
	DefaultNotifyBankTransferStaleSec = 43200 // notify.bank_transfer_stale_sec  — 12 hours
	DefaultNotifyWithdrawalStaleSec   = 86400 // notify.withdrawal_stale_sec     — 24 hours

	// Queue-depth alert: number of pending bank-transfer proofs that triggers a
	// P0 EventAdminBankTransferQueueDepth alert alongside the regular pending reminder.
	DefaultNotifyBankTransferQueueDepthThreshold = 20 // notify.bank_transfer_queue_depth_threshold

	// High-value withdrawal: amount in cents above which EventAdminHighValueWithdrawal
	// is also emitted alongside the regular EventAdminWithdrawalPending.
	DefaultNotifyHighValueWithdrawalCents = 1_000_000 // notify.high_value_withdrawal_cents — Q10 000

	// Periodic reminder: interval in seconds between repeated admin "pending" alerts
	// while an approval is still outstanding.
	DefaultNotifyPendingReminderIntervalSec = 14400 // notify.pending_reminder_interval_sec — 4 hours

	// Prediction deadline push alerts: how many minutes before kick-off each
	// reminder fires. Two independent lead times allow a 60-min and 15-min nudge.
	DefaultNotifyPredictionDeadlineLeadMin1 = 60  // notify.prediction_deadline_lead_min_1
	DefaultNotifyPredictionDeadlineLeadMin2 = 15  // notify.prediction_deadline_lead_min_2
	DefaultNotifyPredictionMissingLeadMin   = 120 // notify.prediction_missing_lead_min   — 2 hours

	// SSE delivery: interval in seconds between keep-alive heartbeat frames.
	DefaultNotifySSEHeartbeatIntervalSec = 30 // notify.sse_heartbeat_interval_sec

	// Web Push delivery TTL: how long (in seconds) the push service should
	// retain an undelivered message.
	DefaultNotifyWebPushTTLSec = 86400 // notify.web_push_ttl_sec — 24 hours

	// Web Push notification asset URLs. Operators can override these via
	// system_params to use a CDN or branding-specific asset without a restart.
	DefaultNotifyPushIconURL  = PushDefaultIcon  // notify.push_icon_url
	DefaultNotifyPushBadgeURL = PushDefaultBadge // notify.push_badge_url

	DefaultNotifySchedulerTimezone = "America/Guatemala" // notify.scheduler_timezone

	// DefaultNotifyTemplateCacheTTLSec is the seconds the notification template
	// in-memory cache is considered fresh before a bulk reload from the database.
	// is_runtime=FALSE: restart required.
	DefaultNotifyTemplateCacheTTLSec = 300 // notify.template_cache_ttl_seconds — 5 minutes

	// Push title and body are truncated to these character counts server-side
	// to prevent silent content loss on Android (clips title@100, body@300).
	// is_runtime=TRUE: changes propagate within the 30 s param cache window.
	DefaultNotifyPushTitleMaxChars = 100 // notify.push_title_max_chars
	DefaultNotifyPushBodyMaxChars  = 300 // notify.push_body_max_chars

	// DefaultNotifyPushSubRetentionDays is the number of days after inactivation
	// before a push subscription row is permanently deleted by the pruning job.
	// is_runtime=TRUE: takes effect on the next daily prune run.
	DefaultNotifyPushSubRetentionDays = 30 // notify.push_sub_retention_days

	// DefaultNotifyPushDigestWindowSec is the sliding-window length (in seconds)
	// used by PushDigestGate. P2/P3 pushes beyond the threshold within this window
	// are collapsed into a single digest push and subsequent ones are dropped.
	// is_runtime=FALSE: worker restart required to change the gate's window.
	DefaultNotifyPushDigestWindowSec = 300 // notify.push_digest_window_sec — 5 minutes

	// DefaultNotifyPushDigestThreshold is the number of individual P2/P3 push
	// notifications that may be delivered within the digest window before the gate
	// collapses further events into a single digest push.
	// is_runtime=FALSE: worker restart required.
	DefaultNotifyPushDigestThreshold = 5 // notify.push_digest_threshold

	// DefaultNotifyRenderTimeoutMs is the wall-clock budget (milliseconds) for a
	// single email template render before the dispatcher returns an error and the
	// outbox worker applies exponential-backoff retry. The default of 5 000 ms
	// (5 s) is generous for pure-CPU template execution; lower it when profiling
	// shows renders complete in <100 ms in production. is_runtime=TRUE.
	DefaultNotifyRenderTimeoutMs = 5_000 // notify.render_timeout_ms

	// Notification outbox worker (is_runtime=FALSE: worker restart required).
	// These constants mirror the outbox.Worker option defaults so that the domain
	// package is the single source of truth for tunable operational values.

	// DefaultNotifyOutboxBatchSize is the maximum number of domain_outbox rows
	// claimed and dispatched per poll cycle.
	DefaultNotifyOutboxBatchSize = 50 // notify.outbox_batch_size

	// DefaultNotifyOutboxPollIntervalSec is the seconds between successive outbox
	// poll cycles. Lower values reduce notification latency at the cost of more
	// idle database round-trips.
	DefaultNotifyOutboxPollIntervalSec = 2 // notify.outbox_poll_interval_sec

	// DefaultNotifyOutboxLockDurationSec is the seconds a claimed outbox row is
	// held before the stale-lock recovery job reclaims it. Must be longer than
	// the worst-case dispatch time (including retries) for a single entry.
	DefaultNotifyOutboxLockDurationSec = 300 // notify.outbox_lock_duration_sec — 5 min

	// DefaultNotifyOutboxMaxAttempts is the maximum dispatch attempts for a single
	// outbox entry before it is marked as permanently failed.
	DefaultNotifyOutboxMaxAttempts = 5 // notify.outbox_max_attempts

	// DefaultNotifyOutboxLagAlertThresholdSec is the outbox lag in seconds above
	// which the worker fires a NotifyOutboxLag alert on each poll cycle.
	DefaultNotifyOutboxLagAlertThresholdSec = 30 // notify.outbox_lag_alert_threshold_sec

	// Notification DLQ replay worker (is_runtime=FALSE: worker restart required).
	// These constants mirror the outbox.DLQWorker option defaults so that the
	// domain package is the single source of truth for tunable operational values.

	// DefaultNotifyDLQReplayBatchSize is the maximum number of notification_dlq
	// entries claimed per poll cycle.
	DefaultNotifyDLQReplayBatchSize = 20 // notify.dlq_replay_batch_size

	// DefaultNotifyDLQReplayPollIntervalSec is the seconds between successive
	// DLQ replay poll cycles. Lower values recover from failures faster at the
	// cost of more idle database round-trips.
	DefaultNotifyDLQReplayPollIntervalSec = 30 // notify.dlq_replay_poll_interval_sec

	// DefaultNotifyDLQReplayMaxAttempts is the maximum number of replay attempts
	// for a single DLQ entry before it is permanently abandoned.
	DefaultNotifyDLQReplayMaxAttempts = 5 // notify.dlq_replay_max_attempts

	// DefaultNotifyDLQReplayAlertThreshold is the unresolved-entry count above
	// which the DLQ worker fires an n8n overflow alert on each poll cycle.
	DefaultNotifyDLQReplayAlertThreshold = 50 // notify.dlq_replay_alert_threshold
)

// Notification system parameter keys.
const (
	// ParamKeyNotifyBankTransferStaleSec is the seconds after which an unreviewed
	// bank-transfer proof triggers EventAdminBankTransferStale.
	ParamKeyNotifyBankTransferStaleSec = "notify.bank_transfer_stale_sec"
	// ParamKeyNotifyWithdrawalStaleSec is the seconds after which an unreviewed
	// withdrawal request triggers EventAdminWithdrawalStale.
	ParamKeyNotifyWithdrawalStaleSec = "notify.withdrawal_stale_sec"
	// ParamKeyNotifyHighValueWithdrawalCents is the threshold in cents above which
	// a withdrawal also triggers EventAdminHighValueWithdrawal.
	ParamKeyNotifyHighValueWithdrawalCents = "notify.high_value_withdrawal_cents"
	// ParamKeyNotifyPendingReminderIntervalSec is the seconds between repeated
	// "pending action required" admin alerts while an operation is still waiting.
	ParamKeyNotifyPendingReminderIntervalSec = "notify.pending_reminder_interval_sec"
	// ParamKeyNotifyPredictionDeadlineLeadMin1 is the first (earlier) push-alert
	// lead time in minutes before prediction deadline closes.
	ParamKeyNotifyPredictionDeadlineLeadMin1 = "notify.prediction_deadline_lead_min_1"
	// ParamKeyNotifyPredictionDeadlineLeadMin2 is the second (later, closer) push-alert
	// lead time in minutes before prediction deadline closes.
	ParamKeyNotifyPredictionDeadlineLeadMin2 = "notify.prediction_deadline_lead_min_2"
	// ParamKeyNotifyPredictionMissingLeadMin is the lead time in minutes before
	// kick-off at which a missing-prediction reminder is sent.
	ParamKeyNotifyPredictionMissingLeadMin = "notify.prediction_missing_lead_min"
	// ParamKeyNotifyBankTransferQueueDepthThreshold is the number of pending
	// bank-transfer proofs that triggers a P0 EventAdminBankTransferQueueDepth alert.
	ParamKeyNotifyBankTransferQueueDepthThreshold = "notify.bank_transfer_queue_depth_threshold"

	// ParamKeyNotifyAdminEmails is a comma-separated list of email addresses that
	// receive all admin.* and system.* notification events.
	ParamKeyNotifyAdminEmails = "notify.admin_emails"
	// ParamKeyNotifyWebPushVAPIDPublicKey is the VAPID public key (base64url-encoded
	// P-256 point) used to sign Web Push subscription requests (RFC 8292).
	ParamKeyNotifyWebPushVAPIDPublicKey = "notify.web_push_vapid_public_key"
	// ParamKeyNotifyWebPushVAPIDSubject is the VAPID subject claim — an HTTPS URL
	// or mailto: address that identifies the application server to push services.
	ParamKeyNotifyWebPushVAPIDSubject = "notify.web_push_vapid_subject"

	// ParamKeyNotifySSEHeartbeatIntervalSec is the interval in seconds between
	// keep-alive heartbeat frames on an open SSE connection.
	ParamKeyNotifySSEHeartbeatIntervalSec = "notify.sse_heartbeat_interval_sec"
	// ParamKeyNotifyWebPushTTLSec is the Web Push message time-to-live in seconds.
	ParamKeyNotifyWebPushTTLSec = "notify.web_push_ttl_sec"
	// ParamKeyNotifyPushIconURL is the URL of the notification icon (192×192 px PNG).
	// is_runtime=TRUE: changes propagate within the 30 s cache window.
	ParamKeyNotifyPushIconURL = "notify.push_icon_url"
	// ParamKeyNotifyPushBadgeURL is the URL of the monochrome badge icon (72×72 px PNG).
	// is_runtime=TRUE: changes propagate within the 30 s cache window.
	ParamKeyNotifyPushBadgeURL = "notify.push_badge_url"
	// ParamKeyNotifySchedulerTimezone is the IANA timezone for the notification
	// scheduler (e.g. "America/Guatemala"). is_runtime=FALSE: worker restart required.
	ParamKeyNotifySchedulerTimezone = "notify.scheduler_timezone"
	// ParamKeyNotifyDefaultLocale is the BCP-47 language tag for all user-facing
	// notification text ("en" or "es"). is_runtime=TRUE.
	ParamKeyNotifyDefaultLocale = "notify.default_locale"
	// ParamKeyNotifyTemplateCacheTTLSec is the seconds the notification template
	// in-memory cache is considered fresh. is_runtime=FALSE: restart required.
	ParamKeyNotifyTemplateCacheTTLSec = "notify.template_cache_ttl_seconds"
	// ParamKeyNotifyPushTitleMaxChars is the maximum rune count for a push title.
	// is_runtime=TRUE: changes propagate within the 30 s param cache window.
	ParamKeyNotifyPushTitleMaxChars = "notify.push_title_max_chars"
	// ParamKeyNotifyPushBodyMaxChars is the maximum rune count for a push body.
	// is_runtime=TRUE: changes propagate within the 30 s param cache window.
	ParamKeyNotifyPushBodyMaxChars = "notify.push_body_max_chars"
	// ParamKeyNotifyPushSubRetentionDays is the days after inactivation before a
	// push subscription is permanently deleted. is_runtime=TRUE.
	ParamKeyNotifyPushSubRetentionDays = "notify.push_sub_retention_days"

	// ParamKeyNotifyFromAddress is the "From:" header value for all outgoing
	// notification emails (user and admin). When set, overrides the
	// WCQ_EMAIL_FROMADDRESS environment variable at send time without a restart.
	// Format: "Display Name <email@example.com>" or bare "email@example.com".
	// is_runtime=TRUE: changes propagate within the 30 s param cache window.
	ParamKeyNotifyFromAddress = "notify.from_address"

	// ParamKeyNotifyPushDigestWindowSec is the duration in seconds of the
	// PushDigestGate's sliding window per user. is_runtime=FALSE: worker restart.
	ParamKeyNotifyPushDigestWindowSec = "notify.push_digest_window_sec"

	// ParamKeyNotifyPushDigestThreshold is the maximum number of individual P2/P3
	// push notifications sent per user per digest window before collapsing to a
	// summary digest. is_runtime=FALSE: worker restart required.
	ParamKeyNotifyPushDigestThreshold = "notify.push_digest_threshold"

	// ParamKeyNotifyRenderTimeoutMs is the maximum milliseconds allowed for a
	// single email template render. Renders that exceed this budget return an error
	// so the outbox worker retries the entry rather than stalling indefinitely.
	// is_runtime=TRUE: new value takes effect within the 30 s param cache window.
	ParamKeyNotifyRenderTimeoutMs = "notify.render_timeout_ms"

	// Notification outbox worker knobs (is_runtime=FALSE: worker restart required).

	// ParamKeyNotifyOutboxBatchSize is the number of domain_outbox entries claimed
	// per poll cycle by the outbox dispatch worker.
	ParamKeyNotifyOutboxBatchSize = "notify.outbox_batch_size"
	// ParamKeyNotifyOutboxPollIntervalSec is the seconds between outbox poll cycles.
	// Increase to reduce idle DB load; decrease to lower notification latency.
	ParamKeyNotifyOutboxPollIntervalSec = "notify.outbox_poll_interval_sec"
	// ParamKeyNotifyOutboxLockDurationSec is the seconds a claimed outbox row is
	// locked before the stale-lock recovery job reclaims it.
	ParamKeyNotifyOutboxLockDurationSec = "notify.outbox_lock_duration_sec"
	// ParamKeyNotifyOutboxMaxAttempts is the maximum dispatch attempts per outbox
	// entry before it is permanently marked failed.
	ParamKeyNotifyOutboxMaxAttempts = "notify.outbox_max_attempts"
	// ParamKeyNotifyOutboxLagAlertThresholdSec is the lag age in seconds above which
	// the outbox worker fires a NotifyOutboxLag alert.
	ParamKeyNotifyOutboxLagAlertThresholdSec = "notify.outbox_lag_alert_threshold_sec"

	// Notification DLQ replay worker knobs (is_runtime=FALSE: worker restart required).

	// ParamKeyNotifyDLQReplayBatchSize is the number of notification_dlq entries
	// claimed per poll cycle by the DLQ replay worker.
	ParamKeyNotifyDLQReplayBatchSize = "notify.dlq_replay_batch_size"
	// ParamKeyNotifyDLQReplayPollIntervalSec is the seconds between DLQ replay
	// poll cycles. Increase to reduce idle DB load; decrease to recover faster.
	ParamKeyNotifyDLQReplayPollIntervalSec = "notify.dlq_replay_poll_interval_sec"
	// ParamKeyNotifyDLQReplayMaxAttempts is the maximum replay attempts per
	// notification_dlq entry before it is permanently abandoned.
	ParamKeyNotifyDLQReplayMaxAttempts = "notify.dlq_replay_max_attempts"
	// ParamKeyNotifyDLQReplayAlertThreshold is the unresolved DLQ count above
	// which an n8n overflow alert is fired on each poll cycle.
	ParamKeyNotifyDLQReplayAlertThreshold = "notify.dlq_replay_alert_threshold"
)
