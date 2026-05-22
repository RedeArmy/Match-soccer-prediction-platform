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

	// Audit write timeout (seconds)
	DefaultAuditWriteTimeoutSeconds = 5 // audit.write_timeout_seconds

	// Audit retry policy
	DefaultAuditMaxRetries   = 2   // audit.max_retries
	DefaultAuditRetryDelayMs = 250 // audit.retry_delay_ms

	// Prediction window
	DefaultPredictionDeadlineMin = 5 // prediction.deadline_minutes — closes predictions 5 min before kick-off

	// Scoring win-method bonuses: 0 = no global bonus; per-phase scoring_rules override this.
	DefaultScoringExtraTimeBonus = 0 // scoring.extra_time_bonus
	DefaultScoringPenaltiesBonus = 0 // scoring.penalties_bonus

	// DB transaction retry policy for transient serialization / deadlock errors.
	// Equal-jitter backoff: attempt 1 → 25–50 ms, attempt 2 → 50–100 ms.
	DefaultTxRetryMaxAttempts = 3    // repository.tx_retry_max_attempts
	DefaultTxRetryBaseDelayMs = 50   // repository.tx_retry_base_delay_ms  (milliseconds)
	DefaultTxRetryMaxDelayMs  = 1000 // repository.tx_retry_max_delay_ms   (milliseconds)
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

	// Infrastructure params — read once at process startup; changes require restart,
	// except cache.leaderboard_ttl_seconds which is propagated immediately via the
	// mutation hook registered in server_compose.go buildHandlers.

	// ParamKeyCacheMatchTTL is the match-list cache TTL in seconds.
	ParamKeyCacheMatchTTL = "cache.match_ttl_seconds"
	// ParamKeyCacheLeaderboardTTL is the leaderboard cache TTL in seconds.
	// is_runtime=TRUE: the mutation hook calls CachedRankingService.UpdateTTL
	// and InvalidateAll so the new value takes effect without a restart.
	ParamKeyCacheLeaderboardTTL = "cache.leaderboard_ttl_seconds"
	// ParamKeyCacheDashboardTTLSeconds is the dashboard stats cache TTL in seconds.
	// is_runtime=TRUE so it can be tuned or set to 0 (disable cache) without restart.
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

	// Audit retry policy (is_runtime=FALSE: process restart required).
	// ParamKeyAuditMaxRetries is the number of write attempts before an audit entry
	// is permanently lost; emits audit_lost=true on exhaustion.
	ParamKeyAuditMaxRetries = "audit.max_retries"
	// ParamKeyAuditRetryDelayMs is the delay in milliseconds between audit write
	// retries to allow transient DB failures to clear.
	ParamKeyAuditRetryDelayMs = "audit.retry_delay_ms"

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

// AllParamKeys returns every ParamKey* string value declared across the domain
// package. It is the single authoritative list used by constraint-coverage tests
// and by cmd/validate-params to verify that every constant is registered and
// seeded.  Order is stable and matches the declaration order in the constants
// files (constants.go, constants_notify.go, constants_worker.go).
func AllParamKeys() []string {
	return []string{
		// Scoring
		ParamKeyScoringExactScore,
		ParamKeyScoringCorrectOutcome,
		ParamKeyScoringGoalDiff,
		ParamKeyScoringExtraTimeBonus,
		ParamKeyScoringPenaltiesBonus,
		// Prediction
		ParamKeyPredictionDeadlineMin,
		// Group
		ParamKeyGroupMinMembers,
		ParamKeyGroupMaxSize,
		ParamKeyGroupInviteCodeLength,
		// Conflict
		ParamKeyConflictStaleDays,
		ParamKeyConflictMaxScan,
		// Pagination
		ParamKeyPaginationDefaultLimit,
		ParamKeyPaginationMaxLimit,
		// Tournament
		ParamKeyTournamentWinPoints,
		// Admin
		ParamKeyAdminBulkMaxItems,
		// Cache
		ParamKeyCacheMatchTTL,
		ParamKeyCacheLeaderboardTTL,
		ParamKeyCacheDashboardTTLSeconds,
		// Audit
		ParamKeyAuditWriteTimeout,
		ParamKeyAuditMaxRetries,
		ParamKeyAuditRetryDelayMs,
		// Auth
		ParamKeyAuthValidationTimeout,
		// DLQ
		ParamKeyDLQSampleSize,
		ParamKeyDLQReplayDefaultLimit,
		// Messaging
		ParamKeyMessagingMaxRetries,
		ParamKeyMessagingStreamMaxLen,
		ParamKeyMessagingStreamWorkerCount,
		ParamKeyMessagingStreamReadBlockSec,
		// Worker
		ParamKeyWorkerSnapshotConcurrency,
		ParamKeyWorkerSnapshotRetryBaseMs,
		ParamKeyWorkerSnapshotMaxAttempts,
		ParamKeyWorkerDLQMonitorIntervalSec,
		ParamKeyWorkerPurgeIntervalHours,
		ParamKeyWorkerSchedPredDeadlineIntervalSec,
		ParamKeyWorkerSchedMatchResultIntervalSec,
		ParamKeyWorkerSchedPendingReminderIntervalSec,
		ParamKeyWorkerSchedStaleEscalationIntervalSec,
		ParamKeyWorkerSchedPushPruneIntervalSec,
		// System
		ParamKeyPurgeRetentionDays,
		ParamKeySystemParamHistoryRetentionDays,
		// API
		ParamKeyAPIBodySizeLimitBytes,
		ParamKeyAPIRateLimitRatePerSec,
		ParamKeyAPIRateLimitBurst,
		ParamKeyAPIIdempotencyTTLHours,
		ParamKeyAPIIdempotencyKeyMaxLen,
		// Snapshot
		ParamKeySnapshotKeepLatestCount,
		// Circuit breaker
		ParamKeyBreakerPaypalCertMaxFails,
		ParamKeyBreakerPaypalCertCooldownSec,
		ParamKeyBreakerFileStoreMaxFails,
		ParamKeyBreakerFileStoreCooldownSec,
		// Repository / TX retry
		ParamKeyTxRetryMaxAttempts,
		ParamKeyTxRetryBaseDelayMs,
		ParamKeyTxRetryMaxDelayMs,
		// Payment
		ParamKeyPaymentMaxUploadBytes,
		ParamKeyWithdrawalMinCents,
		ParamKeyWithdrawalMaxCents,
		ParamKeyBankTransferMinAmountCents,
		ParamKeyBankTransferMaxAmountCents,
		ParamKeyPaymentIntentTTLMinutes,
		// Notification subsystem (constants_notify.go)
		ParamKeyNotifyBankTransferStaleSec,
		ParamKeyNotifyWithdrawalStaleSec,
		ParamKeyNotifyHighValueWithdrawalCents,
		ParamKeyNotifyPendingReminderIntervalSec,
		ParamKeyNotifyPredictionDeadlineLeadMin1,
		ParamKeyNotifyPredictionDeadlineLeadMin2,
		ParamKeyNotifyPredictionMissingLeadMin,
		ParamKeyNotifyBankTransferQueueDepthThreshold,
		ParamKeyNotifyAdminEmails,
		ParamKeyNotifyWebPushVAPIDPublicKey,
		ParamKeyNotifyWebPushVAPIDSubject,
		ParamKeyNotifySSEHeartbeatIntervalSec,
		ParamKeyNotifyWebPushTTLSec,
		ParamKeyNotifyPushIconURL,
		ParamKeyNotifyPushBadgeURL,
		ParamKeyNotifySchedulerTimezone,
		ParamKeyNotifyDefaultLocale,
		ParamKeyNotifyTemplateCacheTTLSec,
		ParamKeyNotifyPushTitleMaxChars,
		ParamKeyNotifyPushBodyMaxChars,
		ParamKeyNotifyPushSubRetentionDays,
		ParamKeyNotifyFromAddress,
		ParamKeyNotifyPushDigestWindowSec,
		ParamKeyNotifyPushDigestThreshold,
	}
}
