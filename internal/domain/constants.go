package domain

import "time"

// Scoring rules define the points awarded per prediction outcome.
//
// The full matrix, from best to worst:
//
//	PointsExactScore (5)
//	  — player predicts the exact scoreline (e.g. 2–1 → 2–1).
//
//	PointsCorrectOutcome (2) [+ PointsGoalDifference (1) when applicable]
//	  — player predicts the correct winner or draw but not the exact score.
//	  — an extra PointsGoalDifference point is added when the predicted
//	    goal margin equals the actual margin (e.g. predict 2–0, result 3–1:
//	    both are a 2-goal home win). This bonus does NOT apply to draws
//	    because every draw has a goal difference of 0, making the check
//	    trivially true and not a meaningful prediction skill.
//
//	PointsIncorrectResult (0)
//	  — player predicts the wrong outcome, or no prediction was submitted.
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

// DefaultPrizeThreshold is applied by QuinielaService.Create when the caller
// does not supply a PrizeThreshold. The prize distribution formula is:
//
//	winnerCount = max(1, floor(memberCount / PrizeThreshold))
//
// With a threshold of 3, a 9-member group has 3 prize winners and a 2-member
// group always has at least 1. The value must stay consistent with the DEFAULT
// clause in migration 000023_add_prize_threshold_to_quinielas.up.sql.
const DefaultPrizeThreshold = 3

// StandingsWinPoints is the number of standing points awarded to the winner of
// a group-stage match, per the FIFA 3-point rule. Used by TournamentService as
// the fallback when the tournament.win_points system param is absent.
const StandingsWinPoints = 3

// MinMembersForActive is the minimum number of active members a quiniela must
// have for the system to set its status to QuinielaStatusActive. Groups below
// this threshold are QuinielaStatusInactive: predictions can still be submitted
// but the group is not eligible for payment processing or prize distribution.
//
// The value is 3 — a group of 1 (owner only) or 2 cannot be active. This
// constant is the single source of truth referenced by GroupMembershipService;
// changing it here is sufficient to adjust the threshold system-wide.
const MinMembersForActive = 3

// DefaultConflictStaleDays is the fallback staleness threshold used by
// ConflictService when ParamKeyConflictStaleDays is absent from system_params.
// At runtime, the actual value is always read from the param table; this
// constant is only the safe default, not the authoritative runtime value.
const DefaultConflictStaleDays = 7

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
	ParamKeyGroupDefaultPrize     = "group.default_prize_threshold"
	// ParamKeyGroupInviteCodeLength is the number of characters in a generated
	// invite code. Defaults to inviteCodeLength (10) in QuinielaService.
	ParamKeyGroupInviteCodeLength = "group.invite_code_length"
	// ParamKeyConflictStaleDays is the age in days after which a pending payment
	// or membership is flagged as a conflict. Defaults to DefaultConflictStaleDays (7).
	ParamKeyConflictStaleDays = "conflict.stale_days"
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

	// Infrastructure params — read once at process startup; changes require restart.
	// The is_runtime column in system_params is set to FALSE for all of these.

	// ParamKeyCacheMatchTTL is the match-list cache TTL in seconds.
	ParamKeyCacheMatchTTL = "cache.match_ttl_seconds"
	// ParamKeyCacheLeaderboardTTL is the leaderboard cache TTL in seconds.
	ParamKeyCacheLeaderboardTTL = "cache.leaderboard_ttl_seconds"
	// ParamKeyCacheDashboardTTLSeconds is the dashboard stats cache TTL in seconds.
	// Defaults to 30 s — long enough to absorb repeated dashboard loads but short
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
	// ParamKeyAuthValidationTimeout is the JWKS warm-up timeout in seconds.
	ParamKeyAuthValidationTimeout = "auth.validation_timeout_seconds"
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
)

// AllMatchPhases is the ordered list of every tournament phase defined in the
// MatchPhase type. It is the single source of truth for any code that must
// iterate over all phases — for example, cache invalidation, report generation,
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
