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

// MinMembersForActive is the minimum number of active members a quiniela must
// have for the system to set its status to QuinielaStatusActive. Groups below
// this threshold are QuinielaStatusInactive: predictions can still be submitted
// but the group is not eligible for payment processing or prize distribution.
//
// The value is 3 — a group of 1 (owner only) or 2 cannot be active. This
// constant is the single source of truth referenced by GroupMembershipService;
// changing it here is sufficient to adjust the threshold system-wide.
const MinMembersForActive = 3

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
