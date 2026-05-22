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
)
