package domain

import "time"

// Input validation length limits enforce application-layer bounds on text fields
// before they reach the database. This prevents DoS attacks via oversized JSON
// payloads and provides fast-fail feedback at the API boundary rather than relying
// on DB truncation errors. These limits apply to all user-supplied text regardless
// of whether the underlying column is VARCHAR(n) or TEXT.
const (
	// MaxEmailLength is the RFC 5321 maximum: 64 (local-part) + 1 (@) + 255 (domain).
	// Enforced by ValidateEmail before any database write or external API call.
	MaxEmailLength = 320

	// MaxNameLength caps user.name and quiniela.name. 200 characters is generous
	// enough for international names and group titles while preventing multi-KB abuse.
	MaxNameLength = 200

	// MaxTeamNameLength caps match.home_team and match.away_team. 100 characters
	// covers the longest real-world team names (e.g. "Borussia Mönchengladbach")
	// with headroom for future FIFA expansions.
	MaxTeamNameLength = 100
)

// Scoring rules define the points awarded per prediction outcome.
//
// The full matrix, from best to worst:
//
//	PointsExactScore (5)
//	  - player predicts the exact scoreline (e.g. 2-1 -> 2-1).
//
//	PointsCorrectOutcome (2) [+ PointsGoalDifference (1) when applicable]
//	  - player predicts the correct winner or draw but not the exact score.
//	  - an extra PointsGoalDifference point is added when the predicted
//	    goal margin equals the actual margin (e.g. predict 2-0, result 3-1:
//	    both are a 2-goal home win). This bonus does NOT apply to draws
//	    because every draw has a goal difference of 0, making the check
//	    trivially true and not a meaningful prediction skill.
//
//	PointsIncorrectResult (0)
//	  - player predicts the wrong outcome, or no prediction was submitted.
//
// These constants are the single source of truth referenced by MatchScorer;
// no other package should hard-code scoring values.
const (
	PointsExactScore      = 5
	PointsCorrectOutcome  = 2
	PointsGoalDifference  = 1 // bonus for correct goal margin on non-draw results
	PointsIncorrectResult = 0
)

// PredictionDeadlineOffset is the duration before kick-off after which
// predictions are no longer accepted.
//
// The deadline is KickoffAt − PredictionDeadlineOffset. Any submission or
// update that arrives after that moment is rejected by domain.ValidatePrediction.
// Five minutes gives the system time to process last-second changes while
// preventing players from adjusting predictions once team news (e.g. a
// goalkeeper injury) becomes public in the stadium.
const PredictionDeadlineOffset = 5 * time.Minute

// StandingsWinPoints is the number of standing points awarded to the winner of
// a group-stage match, per the FIFA 3-point rule. Used by TournamentService as
// the fallback when the tournament.win_points system param is absent.
const StandingsWinPoints = 3

// MinMembersPerGroup is the minimum number of active paid members a quiniela
// must reach before it is eligible for payment processing and prize
// distribution. Groups below this count remain QuinielaStatusInactive.
// Matches the lowest prize tier threshold in WinnerCount.
const MinMembersPerGroup = 5

// MinMembersForActive is an alias for MinMembersPerGroup used by
// GroupMembershipService when reading the runtime system param. The alias
// preserves the existing param key contract without a migration.
const MinMembersForActive = MinMembersPerGroup

// MaxMembersPerGroup is the fallback cap on active members per quiniela used
// when the group.max_size system param is absent or unparseable. The
// authoritative runtime value is always read from system_params via
// ParamKeyGroupMaxSize; this constant is the safe default only.
const MaxMembersPerGroup = 20

// DefaultConflictStaleDays is the fallback staleness threshold used by
// ConflictService when ParamKeyConflictStaleDays is absent from system_params.
// At runtime, the actual value is always read from the param table; this
// constant is only the safe default, not the authoritative runtime value.
const DefaultConflictStaleDays = 7

// DefaultConflictMaxScan caps the number of conflicts loaded into memory during
// internal scan operations (ConflictSummary). Under normal operation this limit
// should never be hit - 5000 unresolved conflicts indicates a systemic issue.
// The cap prevents unbounded memory growth when ConflictSummary is called by
// background jobs or dashboard widgets. Admin endpoints that paginate conflict
// lists (ListConflicts via GET /admin/conflicts) are unaffected by this limit.
const DefaultConflictMaxScan = 5000

// Default values for system parameters. Each constant is the fallback used when
// the corresponding ParamKey row is absent or unparseable. They are the single
// source of truth for numeric defaults: the same values must match the seed data
// in migrations/000040_seed_system_params.up.sql and its follow-up migrations.
const (
	// Group configuration
	DefaultGroupInviteCodeLength = 10 // group.invite_code_length

	// Pagination
	DefaultPaginationDefaultLimit = 50  // pagination.default_limit
	DefaultPaginationMaxLimit     = 200 // pagination.max_limit

	// Admin bulk operations
	DefaultAdminBulkMaxItems = 1000 // admin.bulk_max_items

	// Cache TTLs (seconds)
	DefaultCacheDashboardTTLSeconds   = 30  // cache.dashboard_ttl_seconds
	DefaultCacheMatchTTLSeconds       = 300 // cache.match_ttl_seconds
	DefaultCacheLeaderboardTTLSeconds = 60  // cache.leaderboard_ttl_seconds

	// Dead-letter queue
	DefaultDLQSampleSize         = 5  // dlq.sample_size
	DefaultDLQReplayDefaultLimit = 10 // dlq.replay_default_limit

	// Messaging / Redis Streams
	DefaultMessagingMaxRetries         = 3       // messaging.max_retries
	DefaultMessagingStreamMaxLen       = 600_000 // messaging.stream_max_len
	DefaultMessagingStreamWorkerCount  = 8       // messaging.stream_worker_count
	DefaultMessagingStreamReadBlockSec = 5       // messaging.stream_read_block_sec

	// Infrastructure timeouts (seconds)
	DefaultAuthValidationTimeoutSeconds = 5 // auth.validation_timeout_seconds
	DefaultAuditWriteTimeoutSeconds     = 5 // audit.write_timeout_seconds

	// Audit retry policy
	DefaultAuditMaxRetries   = 2   // audit.max_retries
	DefaultAuditRetryDelayMs = 250 // audit.retry_delay_ms

	// Worker: leaderboard snapshot generation
	// 4 concurrent snapshots is sized for a single shared-CPU machine with 256 MB RAM.
	// Each goroutine holds pgx row buffers while executing ranking queries; 16 goroutines
	// at peak can exhaust the heap on a memory-constrained instance. Raise via system
	// param worker.snapshot_concurrency when running on a larger instance.
	DefaultWorkerSnapshotConcurrency = 4   // worker.snapshot_concurrency
	DefaultWorkerSnapshotRetryBaseMs = 100 // worker.snapshot_retry_base_ms
	DefaultWorkerSnapshotMaxAttempts = 3   // worker.snapshot_max_attempts

	// Worker: background maintenance jobs
	DefaultWorkerDLQMonitorIntervalSec = 300 // worker.dlq_monitor_interval_sec (5 min)
	DefaultWorkerPurgeIntervalHours    = 24  // worker.purge_interval_hours

	// Soft-delete retention
	DefaultPurgeRetentionDays = 30 // system.purge_retention_days

	// Leaderboard snapshot retention: number of most-recent snapshots to keep per
	// quiniela. The daily purge job deletes every snapshot beyond this count,
	// bounding table growth to (active_quinielas × keep_latest_count) rows.
	// Five snapshots cover the last five match results — sufficient for trend
	// display — while staying well below the 6 400-row worst case for 64 matches
	// across 100 quinielas.
	DefaultSnapshotKeepLatestCount = 5 // snapshot.keep_latest_count

	// API request limits
	DefaultAPIBodySizeLimitBytes = 65536 // api.body_size_limit_bytes (64 KB)

	// API rate limiting: per-user token bucket applied at the /api/v1 subrouter.
	// 10 tokens/second with a burst of 30 allows short activity spikes (e.g.
	// loading a dashboard that issues several parallel requests) while preventing
	// sustained high-frequency polling. Both values are read once at process
	// startup (is_runtime=FALSE); a restart is required to change them.
	DefaultAPIRateLimitRatePerSec = 10 // api.rate_limit_rate_per_sec (tokens/second)
	DefaultAPIRateLimitBurst      = 30 // api.rate_limit_burst (max burst size)

	// Prediction window
	DefaultPredictionDeadlineMin = 5 // prediction.deadline_minutes — closes predictions 5 min before kick-off

	// Scoring win-method bonuses: 0 = no global bonus; per-phase scoring_rules override this.
	DefaultScoringExtraTimeBonus = 0 // scoring.extra_time_bonus
	DefaultScoringPenaltiesBonus = 0 // scoring.penalties_bonus

	// Payment / balance params
	DefaultPaymentMaxUploadBytes = 5_242_880 // payment.max_upload_bytes (5 MB)
	DefaultWithdrawalMinCents    = 5_000     // payment.withdrawal_min_cents (50 GTQ)
	DefaultWithdrawalMaxCents    = 500_000   // payment.withdrawal_max_cents (5 000 GTQ)

	// Bank transfer amount bounds. These mirror the withdrawal limits: the
	// declared amount on a bank transfer proof is validated against these
	// before the proof is accepted for admin review. Prevents claims of
	// unreasonably small or arbitrarily large transfers.
	DefaultBankTransferMinAmountCents = 1_000      // payment.bank_transfer_min_amount_cents (10 GTQ)
	DefaultBankTransferMaxAmountCents = 10_000_000 // payment.bank_transfer_max_amount_cents (100 000 GTQ)

	// DefaultPaymentIntentTTLMinutes is the number of minutes a pending PayPal
	// payment intent remains valid. After this window expires the intent cannot
	// be captured, so the customer must start the checkout flow again. 60 minutes
	// is long enough for a typical PayPal checkout session while being short
	// enough to limit the window for stale captures.
	DefaultPaymentIntentTTLMinutes = 60 // payment.intent_ttl_minutes

	// Idempotency middleware: applied to payment write endpoints.
	// TTL of 24 h gives clients a generous window for safe retry; key length
	// of 255 bytes fits a UUID, hash, or arbitrary client-generated string.
	DefaultAPIIdempotencyTTLHours  = 24  // api.idempotency_ttl_hours
	DefaultAPIIdempotencyKeyMaxLen = 255 // api.idempotency_key_max_len

	// Circuit breaker: PayPal certificate fetcher.
	// Opens after 3 consecutive cert-download failures; stays open for 60 s.
	// PayPal will retry webhook delivery while the circuit is open.
	DefaultBreakerPaypalCertMaxFails    = 3  // breaker.paypal_cert_max_fails
	DefaultBreakerPaypalCertCooldownSec = 60 // breaker.paypal_cert_cooldown_sec

	// Circuit breaker: file store (S3/GDrive/OneDrive).
	// Opens after 5 consecutive storage errors; stays open for 30 s.
	// Handlers return 500 immediately rather than waiting for a network timeout.
	DefaultBreakerFileStoreMaxFails    = 5  // breaker.file_store_max_fails
	DefaultBreakerFileStoreCooldownSec = 30 // breaker.file_store_cooldown_sec

	// DB transaction retry policy for transient serialization / deadlock errors.
	// Equal-jitter backoff: attempt 1 → 25–50 ms, attempt 2 → 50–100 ms.
	DefaultTxRetryMaxAttempts = 3    // repository.tx_retry_max_attempts
	DefaultTxRetryBaseDelayMs = 50   // repository.tx_retry_base_delay_ms  (milliseconds)
	DefaultTxRetryMaxDelayMs  = 1000 // repository.tx_retry_max_delay_ms   (milliseconds)

	// Notification subsystem thresholds (is_runtime=TRUE; changes propagate within cache window).
	// Stale-alert timers: outbox-worker or background job flags a pending operation as stale
	// once it has waited longer than these thresholds without an admin action.
	DefaultNotifyBankTransferStaleSec = 43200 // notify.bank_transfer_stale_sec  — 12 hours
	DefaultNotifyWithdrawalStaleSec   = 86400 // notify.withdrawal_stale_sec     — 24 hours

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

	// SSE delivery parameters (Phase 2).
	// DefaultNotifySSEHeartbeatIntervalSec is the interval in seconds between
	// keep-alive heartbeat frames sent on an open SSE connection.  A shorter
	// interval detects proxy timeouts faster; a longer one reduces server load.
	DefaultNotifySSEHeartbeatIntervalSec = 30 // notify.sse_heartbeat_interval_sec

	// Web Push delivery TTL: how long (in seconds) the push service should
	// retain an undelivered message.  After this window the message is discarded
	// rather than delivered to a reconnected device.
	DefaultNotifyWebPushTTLSec = 86400 // notify.web_push_ttl_sec — 24 hours
)

// System parameter keys used by the service layer to fetch runtime-configurable
// values from SystemParamRepository. Each constant names the row that must
// exist in system_params (seeded by migrations). Services fall back to the
// corresponding domain constant when the key is absent or the value is
// unparseable, so the system degrades gracefully rather than refusing requests.
const (
	ParamKeyScoringExactScore     = "scoring.exact_score"
	ParamKeyScoringCorrectOutcome = "scoring.correct_outcome"
	ParamKeyScoringGoalDiff       = "scoring.goal_difference"
	ParamKeyScoringExtraTimeBonus = "scoring.extra_time_bonus"
	ParamKeyScoringPenaltiesBonus = "scoring.penalties_bonus"
	// ParamKeyPredictionDeadlineMin is the prediction deadline offset in minutes.
	// A value of 5 closes predictions 5 minutes before kick-off.
	ParamKeyPredictionDeadlineMin = "prediction.deadline_minutes"
	ParamKeyGroupMinMembers       = "group.min_members_for_active"
	// ParamKeyGroupMaxSize is the maximum number of active members allowed per
	// quiniela. Enforced by the application layer on join and approval.
	// Defaults to MaxMembersPerGroup (20).
	ParamKeyGroupMaxSize = "group.max_size"
	// ParamKeyGroupInviteCodeLength is the number of characters in a generated
	// invite code. Defaults to DefaultGroupInviteCodeLength (10).
	ParamKeyGroupInviteCodeLength = "group.invite_code_length"
	// ParamKeyConflictStaleDays is the age in days after which a pending payment
	// or membership is flagged as a conflict. Defaults to DefaultConflictStaleDays (7).
	ParamKeyConflictStaleDays = "conflict.stale_days"
	// ParamKeyConflictMaxScan caps the number of conflicts loaded into memory
	// by ConflictSummary. Prevents unbounded memory usage when invoked by background
	// jobs or dashboard endpoints. Defaults to DefaultConflictMaxScan (5000).
	ParamKeyConflictMaxScan = "conflict.max_scan"
	// ParamKeyPaginationDefaultLimit and ParamKeyPaginationMaxLimit control the
	// default and maximum page sizes returned by paginated admin endpoints.
	ParamKeyPaginationDefaultLimit = "pagination.default_limit"
	ParamKeyPaginationMaxLimit     = "pagination.max_limit"

	// ParamKeyTournamentWinPoints is the standing points awarded for a group-stage
	// win. Defaults to StandingsWinPoints (3). Read dynamically by TournamentService.
	ParamKeyTournamentWinPoints = "tournament.win_points"
	// ParamKeyAdminBulkMaxItems is the maximum number of IDs accepted in a single
	// bulk admin operation (BulkDeleteGroups, BulkRemoveMembers). Requests that
	// exceed this limit are rejected with 422 to prevent oversized ANY($1) queries.
	// Read dynamically per request so it can be lowered during high-load periods
	// without a process restart. Defaults to 1000.
	ParamKeyAdminBulkMaxItems = "admin.bulk_max_items"

	// Infrastructure params - read once at process startup; changes require restart,
	// except cache.leaderboard_ttl_seconds which is propagated immediately via the
	// mutation hook registered in server.go buildHandlers.

	// ParamKeyCacheMatchTTL is the match-list cache TTL in seconds.
	ParamKeyCacheMatchTTL = "cache.match_ttl_seconds"
	// ParamKeyCacheLeaderboardTTL is the leaderboard cache TTL in seconds.
	// is_runtime = TRUE: the mutation hook calls CachedRankingService.UpdateTTL
	// and InvalidateAll so the new value takes effect without a restart.
	ParamKeyCacheLeaderboardTTL = "cache.leaderboard_ttl_seconds"
	// ParamKeyCacheDashboardTTLSeconds is the dashboard stats cache TTL in seconds.
	// Defaults to 30 s - long enough to absorb repeated dashboard loads but short
	// enough that aggregate counts stay reasonably fresh. is_runtime = TRUE so it
	// can be tuned or set to 0 (disable cache) without a process restart.
	ParamKeyCacheDashboardTTLSeconds = "cache.dashboard_ttl_seconds"
	// ParamKeyAuditWriteTimeout is the maximum time in seconds the audit log
	// goroutine waits to persist an entry before giving up.
	ParamKeyAuditWriteTimeout = "audit.write_timeout_seconds"
	// ParamKeyDLQSampleSize is the maximum number of DLQ entries returned in
	// the Stats sample field.
	ParamKeyDLQSampleSize = "dlq.sample_size"
	// ParamKeyDLQReplayDefaultLimit is the default number of entries replayed
	// when the caller does not supply an explicit limit.
	ParamKeyDLQReplayDefaultLimit = "dlq.replay_default_limit"
	// ParamKeyMessagingMaxRetries is the total handler attempts before an event
	// is dead-lettered.
	ParamKeyMessagingMaxRetries = "messaging.max_retries"
	// ParamKeyMessagingStreamMaxLen caps Redis Stream length (MAXLEN ~).
	ParamKeyMessagingStreamMaxLen = "messaging.stream_max_len"
	// ParamKeyMessagingStreamWorkerCount is the size of the per-EventType goroutine
	// pool that processes stream messages concurrently. is_runtime=FALSE: restart required.
	ParamKeyMessagingStreamWorkerCount = "messaging.stream_worker_count"
	// ParamKeyMessagingStreamReadBlockSec is the XREADGROUP block timeout in seconds.
	// A smaller value makes the consumer loop react faster to shutdown signals at the
	// cost of more idle Redis round-trips. is_runtime=FALSE: restart required.
	ParamKeyMessagingStreamReadBlockSec = "messaging.stream_read_block_sec"

	// ParamKeyAuthValidationTimeout is the JWKS warm-up timeout in seconds.
	ParamKeyAuthValidationTimeout = "auth.validation_timeout_seconds"

	// Audit retry policy (is_runtime=FALSE: process restart required).
	// ParamKeyAuditMaxRetries is the number of write attempts before an audit entry
	// is permanently lost; emits audit_lost=true on exhaustion.
	ParamKeyAuditMaxRetries = "audit.max_retries"
	// ParamKeyAuditRetryDelayMs is the delay in milliseconds between audit write
	// retries to allow transient DB failures to clear.
	ParamKeyAuditRetryDelayMs = "audit.retry_delay_ms"

	// Worker params (all is_runtime=FALSE: worker restart required).
	// ParamKeyWorkerSnapshotConcurrency caps concurrent quiniela snapshots per event.
	ParamKeyWorkerSnapshotConcurrency = "worker.snapshot_concurrency"
	// ParamKeyWorkerSnapshotRetryBaseMs is the initial snapshot retry backoff in ms;
	// doubles on each subsequent attempt (exponential).
	ParamKeyWorkerSnapshotRetryBaseMs = "worker.snapshot_retry_base_ms"
	// ParamKeyWorkerSnapshotMaxAttempts is the maximum snapshot write attempts per
	// quiniela per match event.
	ParamKeyWorkerSnapshotMaxAttempts = "worker.snapshot_max_attempts"
	// ParamKeyWorkerDLQMonitorIntervalSec is the seconds between DLQ size log events.
	ParamKeyWorkerDLQMonitorIntervalSec = "worker.dlq_monitor_interval_sec"
	// ParamKeyWorkerPurgeIntervalHours is the hours between permanent purge runs.
	ParamKeyWorkerPurgeIntervalHours = "worker.purge_interval_hours"

	// ParamKeyPurgeRetentionDays is the age in days after which soft-deleted
	// users and quinielas are permanently removed by the worker purge goroutine.
	// is_runtime = FALSE: changing the value requires a worker restart.
	ParamKeyPurgeRetentionDays = "system.purge_retention_days"

	// ParamKeyPaymentMaxUploadBytes is the maximum size in bytes for bank transfer
	// proof uploads.
	ParamKeyPaymentMaxUploadBytes = "payment.max_upload_bytes"
	// ParamKeyWithdrawalMinCents is the minimum withdrawal amount in minor units.
	ParamKeyWithdrawalMinCents = "payment.withdrawal_min_cents"
	// ParamKeyWithdrawalMaxCents is the maximum withdrawal amount in minor units.
	ParamKeyWithdrawalMaxCents = "payment.withdrawal_max_cents"
	// ParamKeyBankTransferMinAmountCents is the minimum declared amount in minor
	// units for a bank transfer proof submission.
	// Defaults to DefaultBankTransferMinAmountCents (1 000 = 10 GTQ).
	ParamKeyBankTransferMinAmountCents = "payment.bank_transfer_min_amount_cents"
	// ParamKeyBankTransferMaxAmountCents is the maximum declared amount in minor
	// units for a bank transfer proof submission.
	// Defaults to DefaultBankTransferMaxAmountCents (10 000 000 = 100 000 GTQ).
	ParamKeyBankTransferMaxAmountCents = "payment.bank_transfer_max_amount_cents"
	// ParamKeyPaymentIntentTTLMinutes is the number of minutes a pending PayPal
	// payment intent remains valid. After expiry the webhook returns NotFound and
	// the customer must restart checkout. is_runtime=TRUE: tunable without restart.
	// Defaults to DefaultPaymentIntentTTLMinutes (60).
	ParamKeyPaymentIntentTTLMinutes = "payment.intent_ttl_minutes"

	// ParamKeyAPIBodySizeLimitBytes is the maximum request body size in bytes.
	// Requests exceeding this limit are rejected with 413 to prevent DoS.
	// is_runtime=FALSE: process restart required.
	ParamKeyAPIBodySizeLimitBytes = "api.body_size_limit_bytes"

	// ParamKeyAPIRateLimitRatePerSec is the token-bucket refill rate in tokens per
	// second applied to each authenticated user on the /api/v1 subrouter.
	// is_runtime=FALSE: the LimiterStore is constructed once at startup; a process
	// restart is required to apply a new rate.
	ParamKeyAPIRateLimitRatePerSec = "api.rate_limit_rate_per_sec"

	// ParamKeyAPIRateLimitBurst is the maximum burst size of the per-user token
	// bucket. A burst of 30 allows up to 30 back-to-back requests before the
	// steady-state rate takes effect. is_runtime=FALSE: restart required.
	ParamKeyAPIRateLimitBurst = "api.rate_limit_burst"

	// ParamKeyAPIIdempotencyTTLHours is the number of hours a committed
	// idempotency entry is retained in the store. Clients may safely retry
	// with the same Idempotency-Key for this duration after the original request.
	// is_runtime=FALSE: the TTL is passed to the store at server startup.
	ParamKeyAPIIdempotencyTTLHours = "api.idempotency_ttl_hours"

	// ParamKeyAPIIdempotencyKeyMaxLen is the maximum byte length of a client-
	// supplied Idempotency-Key header value. Keys exceeding this limit are
	// rejected with 422. is_runtime=FALSE: restart required.
	ParamKeyAPIIdempotencyKeyMaxLen = "api.idempotency_key_max_len"

	// ParamKeySnapshotKeepLatestCount is the number of most-recent leaderboard
	// snapshots to retain per quiniela. The daily purge job deletes every snapshot
	// beyond this count. is_runtime=FALSE: worker restart required.
	ParamKeySnapshotKeepLatestCount = "snapshot.keep_latest_count"

	// Circuit breaker: PayPal certificate fetcher (is_runtime=FALSE: restart required).
	// ParamKeyBreakerPaypalCertMaxFails is the number of consecutive cert-fetch
	// failures before the circuit opens.
	ParamKeyBreakerPaypalCertMaxFails = "breaker.paypal_cert_max_fails"
	// ParamKeyBreakerPaypalCertCooldownSec is the seconds the circuit stays open
	// before allowing a single trial request.
	ParamKeyBreakerPaypalCertCooldownSec = "breaker.paypal_cert_cooldown_sec"

	// Circuit breaker: file store (S3/GDrive/OneDrive) (is_runtime=FALSE: restart required).
	// ParamKeyBreakerFileStoreMaxFails is the number of consecutive storage errors
	// before the file-store circuit opens.
	ParamKeyBreakerFileStoreMaxFails = "breaker.file_store_max_fails"
	// ParamKeyBreakerFileStoreCooldownSec is the seconds the file-store circuit
	// stays open before allowing a single trial request.
	ParamKeyBreakerFileStoreCooldownSec = "breaker.file_store_cooldown_sec"

	// DB transaction retry policy (is_runtime=FALSE: restart required).
	// ParamKeyTxRetryMaxAttempts is the total number of transaction attempts
	// (including the initial one) before a transient error is returned to the caller.
	ParamKeyTxRetryMaxAttempts = "repository.tx_retry_max_attempts"
	// ParamKeyTxRetryBaseDelayMs is the base backoff delay in milliseconds between
	// retry attempts (equal-jitter: actual delay is base/2 + rand[0, base/2]).
	ParamKeyTxRetryBaseDelayMs = "repository.tx_retry_base_delay_ms"
	// ParamKeyTxRetryMaxDelayMs is the maximum backoff delay cap in milliseconds
	// so that very high attempt counts do not wait unreasonably long.
	ParamKeyTxRetryMaxDelayMs = "repository.tx_retry_max_delay_ms"

	// Notification subsystem parameters (is_runtime=TRUE unless noted).

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

	// String params — no integer Default* constant because the value is free-form.

	// ParamKeyNotifyAdminEmails is a comma-separated list of email addresses that
	// receive all admin.* and system.* notification events.
	ParamKeyNotifyAdminEmails = "notify.admin_emails"
	// ParamKeyNotifyWebPushVAPIDPublicKey is the VAPID public key used to sign
	// Web Push subscription requests (RFC 8292). Must be a base64url-encoded P-256 point.
	ParamKeyNotifyWebPushVAPIDPublicKey = "notify.web_push_vapid_public_key"
	// ParamKeyNotifyWebPushVAPIDPrivateKey is the VAPID private key (secret).
	// Keep this value out of source control; inject via environment variable or secrets manager.
	ParamKeyNotifyWebPushVAPIDPrivateKey = "notify.web_push_vapid_private_key"
	// ParamKeyNotifyWebPushVAPIDSubject is the VAPID subject claim — an HTTPS URL
	// or mailto: address that identifies the application server to push services.
	ParamKeyNotifyWebPushVAPIDSubject = "notify.web_push_vapid_subject"

	// ParamKeyNotifySSEHeartbeatIntervalSec is the interval in seconds between
	// keep-alive heartbeat frames on an open SSE connection.
	// Shorter values detect proxy timeouts faster; longer values reduce server load.
	ParamKeyNotifySSEHeartbeatIntervalSec = "notify.sse_heartbeat_interval_sec"
	// ParamKeyNotifyWebPushTTLSec is the Web Push message time-to-live in seconds.
	// The push service discards undelivered messages after this window expires.
	ParamKeyNotifyWebPushTTLSec = "notify.web_push_ttl_sec"
)

// Audit action strings written to the audit_log table. Using constants rather
// than inline strings prevents typos from creating silent mismatches between
// the writer and any downstream query that filters by action.
const (
	AuditActionMatchCreated         = "match.created"
	AuditActionMatchStarted         = "match.started"
	AuditActionMatchResultSet       = "match.result_set"
	AuditActionTiebreakerQuestion   = "tiebreaker.question_set"
	AuditActionTiebreakerResult     = "tiebreaker.result_confirmed"
	AuditActionSlotConfirmed        = "tournament.slot_confirmed"
	AuditActionGroupDeleted         = "admin_group.deleted"
	AuditActionMemberRemoved        = "admin_group.member_removed"
	AuditActionGroupSettingsUpdated = "admin_group.settings_updated"
	AuditActionOwnershipTransferred = "admin_group.ownership_transferred"
	AuditActionUserBanned           = "admin_user.banned"
	AuditActionUserUnbanned         = "admin_user.unbanned"
	AuditActionPaymentCreated       = "payment.created"
	AuditActionPaymentValidated     = "payment.validated"
	AuditActionPaymentRejected      = "payment.rejected"
	AuditActionJoinApproved         = "group.join_approved"
	AuditActionGroupRenamed         = "group.renamed"
	AuditActionParamUpdated         = "param.updated"
	AuditActionConflictAcknowledged = "conflict.acknowledged"
	AuditActionConflictAutoResolved = "conflict.auto_resolved"
	AuditActionMemberBulkRemoved    = "admin_group.member_bulk_removed"
	AuditActionGroupBulkDeleted     = "admin_group.bulk_deleted"
	AuditActionLeaderboardRefreshed = "admin_group.leaderboard_refreshed"
	AuditActionScoringRuleUpdated   = "scoring_rule.updated"

	// Balance and payment actions.
	AuditActionBankTransferUploaded = "bank_transfer.uploaded"
	AuditActionBankTransferApproved = "bank_transfer.approved"
	AuditActionBankTransferRejected = "bank_transfer.rejected"
	AuditActionBalanceCredited      = "balance.credited"
	AuditActionBalanceDebited       = "balance.debited"
	AuditActionWithdrawalRequested  = "withdrawal.requested"
	AuditActionWithdrawalApproved   = "withdrawal.approved"
	AuditActionWithdrawalRejected   = "withdrawal.rejected"
	AuditActionWebhookPaymentCredit = "webhook.payment_credited"
)

// Snapshot schema versions identify the JSONB encoding format used by
// LeaderboardSnapshot.Entries. The repository branches on this version when
// deserialising historical snapshots, allowing the struct layout to evolve
// without breaking reads of older rows.
const (
	// SnapshotSchemaV1 is the original format: Go's default JSON encoding
	// (PascalCase field names, no explicit struct tags).
	SnapshotSchemaV1 = 1
	// SnapshotCurrentSchema is the version written by new snapshots.
	// Advance this constant when the entry struct or encoding changes.
	SnapshotCurrentSchema = SnapshotSchemaV1
)

// AllMatchPhases is the ordered list of every tournament phase defined in the
// MatchPhase type. It is the single source of truth for any code that must
// iterate over all phases - for example, cache invalidation, report generation,
// or test matrix construction. The slice is ordered by tournament progression;
// do not reorder it without updating dependent consumers.
var AllMatchPhases = [...]MatchPhase{
	PhaseGroupStage,
	PhaseRoundOf32,
	PhaseRoundOf16,
	PhaseQuarterFinal,
	PhaseSemiFinal,
	PhaseThirdPlace,
	PhaseFinal,
}

// IsKnockoutPhase reports whether phase is a knockout (single-elimination) round.
// Win-method bonus points only apply to knockout phases.
func IsKnockoutPhase(phase MatchPhase) bool {
	return phase != PhaseGroupStage
}
