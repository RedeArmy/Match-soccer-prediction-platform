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
	DefaultWorkerSnapshotConcurrency = 16  // worker.snapshot_concurrency
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

	// ParamKeyAPIBodySizeLimitBytes is the maximum request body size in bytes.
	// Requests exceeding this limit are rejected with 413 to prevent DoS.
	// is_runtime=FALSE: process restart required.
	ParamKeyAPIBodySizeLimitBytes = "api.body_size_limit_bytes"

	// ParamKeySnapshotKeepLatestCount is the number of most-recent leaderboard
	// snapshots to retain per quiniela. The daily purge job deletes every snapshot
	// beyond this count. is_runtime=FALSE: worker restart required.
	ParamKeySnapshotKeepLatestCount = "snapshot.keep_latest_count"
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
