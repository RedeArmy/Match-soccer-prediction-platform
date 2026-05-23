// Package notification defines the domain vocabulary for the notification
// subsystem: event types, strongly-typed payload structs, priority levels, and
// the OutboxEntry that carries an event from the originating transaction to the
// delivery worker.
//
// Nothing in this package depends on infrastructure (no pgx, no HTTP, no Redis).
// Types are safe to construct and inspect in unit tests without any external
// setup.
package notification

import (
	"sort"
	"time"
)

// EventType identifies the kind of notification event written to the outbox.
// Using a named string type prevents silent routing failures caused by typos.
type EventType string

// User-facing prediction events.
const (
	EventPredictionConfirmed        EventType = "prediction.confirmed"
	EventPredictionDeadlineApproach EventType = "prediction.deadline_approaching"
	EventPredictionMissingReminder  EventType = "prediction.missing_reminder"
	EventPredictionLocked           EventType = "prediction.locked"
	EventPredictionScored           EventType = "prediction.scored"
)

// User-facing match events.
const (
	EventMatchResultEntered EventType = "match.result_entered"
	EventMatchPostponed     EventType = "match.postponed"
	EventMatchCancelled     EventType = "match.cancelled"
)

// User-facing leaderboard events.
const (
	// EventLeaderboardUpdated is a synthetic signal, not stored in the outbox.
	// The worker publishes it directly to the user_notifications Redis channel
	// after scoring and cache invalidation complete. SSE clients use the
	// action_url field to know which group leaderboard to refetch.
	EventLeaderboardUpdated EventType = "leaderboard.updated"
)

// User-facing group / quiniela events.
const (
	EventGroupJoinRequested        EventType = "group.join_requested"
	EventGroupJoinApproved         EventType = "group.join_approved"
	EventGroupJoinRejected         EventType = "group.join_rejected"
	EventGroupMemberJoined         EventType = "group.member_joined"
	EventGroupMemberLeft           EventType = "group.member_left"
	EventGroupLeaderboardMilestone EventType = "group.leaderboard_milestone"
	EventGroupDisbanded            EventType = "group.disbanded"
	EventGroupDeadline24h          EventType = "group.deadline_24h"
)

// User-facing payment events.
const (
	EventPaymentConfirmed             EventType = "payment.confirmed"
	EventPaymentFailed                EventType = "payment.failed"
	EventPaymentBankTransferSubmitted EventType = "payment.bank_transfer_submitted"
	EventPaymentBankTransferApproved  EventType = "payment.bank_transfer_approved"
	EventPaymentBankTransferRejected  EventType = "payment.bank_transfer_rejected"
	EventPaymentPendingTimeout        EventType = "payment.pending_timeout"
)

// User-facing withdrawal events.
const (
	EventWithdrawalRequested      EventType = "withdrawal.requested"
	EventWithdrawalApproved       EventType = "withdrawal.approved"
	EventWithdrawalRejected       EventType = "withdrawal.rejected"
	EventWithdrawalProcessing     EventType = "withdrawal.processing"
	EventWithdrawalCompleted      EventType = "withdrawal.completed"
	EventWithdrawalFailed         EventType = "withdrawal.failed"
	EventWithdrawalPendingTimeout EventType = "withdrawal.pending_timeout"
)

// User-facing account events.
const (
	EventAccountWelcome         EventType = "account.welcome"
	EventAccountBalanceCredited EventType = "account.balance_credited"
	EventAccountBalanceDebited  EventType = "account.balance_debited"
	EventAccountLowBalance      EventType = "account.low_balance"
)

// Admin action-required events (always deliver via email immediately).
const (
	EventAdminBankTransferPending    EventType = "admin.bank_transfer_pending"
	EventAdminBankTransferStale      EventType = "admin.bank_transfer_stale"
	EventAdminBankTransferQueueDepth EventType = "admin.bank_transfer_queue_depth"
	EventAdminWithdrawalPending      EventType = "admin.withdrawal_pending"
	EventAdminWithdrawalStale        EventType = "admin.withdrawal_stale"
	EventAdminHighValueWithdrawal    EventType = "admin.high_value_withdrawal"
	EventAdminPaymentDispute         EventType = "admin.payment_dispute"
	EventAdminMatchResultPending     EventType = "admin.match_result_pending"
	EventAdminScoringDiscrepancy     EventType = "admin.scoring_discrepancy"
	EventAdminGroupReported          EventType = "admin.group_reported"

	// Scheduler-emitted digest events (Phase 4 — Sprint 7).
	// EventAdminPendingReminder is emitted every 4 hours when pending
	// bank transfers or withdrawal requests await admin action.
	EventAdminPendingReminder EventType = "admin.pending_reminder"
	// EventAdminDailySummary is emitted daily at the configured hour with
	// operational metrics for the preceding 24-hour window.
	EventAdminDailySummary EventType = "admin.daily_summary"
	// EventAdminWeeklyReport is emitted every Monday at the configured hour
	// with revenue, top groups, and top users for the preceding 7 days.
	EventAdminWeeklyReport EventType = "admin.weekly_report"
)

// System / infrastructure alert events (always deliver via email + ops channel).
const (
	EventSystemCircuitBreakerOpened     EventType = "system.circuit_breaker_opened"
	EventSystemCircuitBreakerHalfOpen   EventType = "system.circuit_breaker_half_open"
	EventSystemWebhookSignatureFailed   EventType = "system.webhook_signature_failed"
	EventSystemWebhookSignatureRepeated EventType = "system.webhook_signature_repeated_fail"
	EventSystemTxRetryExhausted         EventType = "system.tx_retry_exhausted"
	EventSystemBalanceLedgerMismatch    EventType = "system.balance_ledger_mismatch"
	EventSystemRateLimitAbuse           EventType = "system.rate_limit_abuse"
	EventSystemIdempotencyCollision     EventType = "system.idempotency_collision"
	EventSystemFileStoreUnavailable     EventType = "system.file_store_unavailable"
)

// ---------------------------------------------------------------------------
// Payload structs — one per event type.
// Each struct carries only the fields needed by the dispatcher and templates.
// ---------------------------------------------------------------------------

// PredictionConfirmedPayload is the payload for EventPredictionConfirmed.
type PredictionConfirmedPayload struct {
	UserID       int    `json:"user_id"`
	PredictionID int    `json:"prediction_id"`
	MatchID      int    `json:"match_id"`
	HomeTeam     string `json:"home_team"`
	AwayTeam     string `json:"away_team"`
}

// PredictionDeadlinePayload is the payload for EventPredictionDeadlineApproach
// and EventPredictionMissingReminder.
type PredictionDeadlinePayload struct {
	UserID      int       `json:"user_id"`
	MatchID     int       `json:"match_id"`
	HomeTeam    string    `json:"home_team"`
	AwayTeam    string    `json:"away_team"`
	DeadlineAt  time.Time `json:"deadline_at"`
	MinutesLeft int       `json:"minutes_left"`
}

// PredictionLockedPayload is the payload for EventPredictionLocked.
type PredictionLockedPayload struct {
	UserID   int    `json:"user_id"`
	MatchID  int    `json:"match_id"`
	HomeTeam string `json:"home_team"`
	AwayTeam string `json:"away_team"`
}

// PredictionScoredPayload is the payload for EventPredictionScored.
type PredictionScoredPayload struct {
	UserID       int    `json:"user_id"`
	PredictionID int    `json:"prediction_id"`
	MatchID      int    `json:"match_id"`
	HomeTeam     string `json:"home_team"`
	AwayTeam     string `json:"away_team"`
	HomeScore    int    `json:"home_score"`
	AwayScore    int    `json:"away_score"`
	PointsEarned int    `json:"points_earned"`
}

// MatchEventPayload is shared by EventMatchResultEntered, EventMatchPostponed,
// and EventMatchCancelled.
type MatchEventPayload struct {
	MatchID   int    `json:"match_id"`
	HomeTeam  string `json:"home_team"`
	AwayTeam  string `json:"away_team"`
	HomeScore *int   `json:"home_score,omitempty"`
	AwayScore *int   `json:"away_score,omitempty"`
}

// GroupJoinPayload is shared by EventGroupJoinRequested, EventGroupJoinApproved,
// EventGroupJoinRejected, EventGroupMemberJoined, and EventGroupMemberLeft.
type GroupJoinPayload struct {
	QuinielaID   int    `json:"quiniela_id"`
	QuinielaName string `json:"quiniela_name"`
	MembershipID int    `json:"membership_id"`
	UserID       int    `json:"user_id"`
	OwnerID      int    `json:"owner_id"`
}

// GroupLeaderboardMilestonePayload is the payload for
// EventGroupLeaderboardMilestone.
type GroupLeaderboardMilestonePayload struct {
	UserID       int    `json:"user_id"`
	QuinielaID   int    `json:"quiniela_id"`
	QuinielaName string `json:"quiniela_name"`
	NewRank      int    `json:"new_rank"`
	TotalPoints  int    `json:"total_points"`
}

// GroupDisbandedPayload is the payload for EventGroupDisbanded.
type GroupDisbandedPayload struct {
	QuinielaID   int    `json:"quiniela_id"`
	QuinielaName string `json:"quiniela_name"`
	OwnerID      int    `json:"owner_id"`
}

// GroupDeadlinePayload is the payload for EventGroupDeadline24h.
type GroupDeadlinePayload struct {
	QuinielaID   int       `json:"quiniela_id"`
	QuinielaName string    `json:"quiniela_name"`
	DeadlineAt   time.Time `json:"deadline_at"`
}

// PaymentPayload is shared by EventPaymentConfirmed and EventPaymentFailed.
type PaymentPayload struct {
	UserID      int    `json:"user_id"`
	PaymentID   int    `json:"payment_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	Reference   string `json:"reference,omitempty"`
	Reason      string `json:"reason,omitempty"` // populated on failure / rejection
}

// BankTransferPayload is shared by EventPaymentBankTransferSubmitted,
// EventPaymentBankTransferApproved, and EventPaymentBankTransferRejected.
type BankTransferPayload struct {
	UserID      int    `json:"user_id"`
	ProofID     int64  `json:"proof_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	AdminID     *int   `json:"admin_id,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// WithdrawalPayload is shared by all withdrawal user events.
type WithdrawalPayload struct {
	UserID      int    `json:"user_id"`
	RequestID   int    `json:"request_id"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	AdminID     *int   `json:"admin_id,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// AccountWelcomePayload is the payload for EventAccountWelcome.
type AccountWelcomePayload struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
	Email    string `json:"email"`
}

// AccountBalancePayload is shared by EventAccountBalanceCredited,
// EventAccountBalanceDebited, and EventAccountLowBalance.
type AccountBalancePayload struct {
	UserID       int    `json:"user_id"`
	AmountCents  int    `json:"amount_cents"`
	BalanceAfter int    `json:"balance_after"`
	Currency     string `json:"currency"`
}

// AdminBankTransferPayload is the payload for admin bank transfer events.
type AdminBankTransferPayload struct {
	ProofID      int64  `json:"proof_id"`
	UserID       int    `json:"user_id"`
	AmountCents  int    `json:"amount_cents"`
	Currency     string `json:"currency"`
	QueueDepth   int    `json:"queue_depth,omitempty"`   // for queue_depth event
	PendingSince string `json:"pending_since,omitempty"` // for stale event
}

// AdminWithdrawalPayload is the payload for admin withdrawal events.
type AdminWithdrawalPayload struct {
	RequestID    int    `json:"request_id"`
	UserID       int    `json:"user_id"`
	AmountCents  int    `json:"amount_cents"`
	Currency     string `json:"currency"`
	IsHighValue  bool   `json:"is_high_value,omitempty"`
	PendingSince string `json:"pending_since,omitempty"` // for stale event
}

// AdminMatchResultPayload is the payload for EventAdminMatchResultPending.
type AdminMatchResultPayload struct {
	MatchID        int       `json:"match_id"`
	HomeTeam       string    `json:"home_team"`
	AwayTeam       string    `json:"away_team"`
	FinishedAt     time.Time `json:"finished_at"`
	MinutesElapsed int       `json:"minutes_elapsed"`
}

// SystemAlertPayload is shared by all system / infrastructure alert events.
type SystemAlertPayload struct {
	Component   string `json:"component"`
	Detail      string `json:"detail"`
	Severity    string `json:"severity"`
	AffectedIDs []int  `json:"affected_ids,omitempty"`
}

// ── Phase 4 scheduler payloads ────────────────────────────────────────────────

// AdminPendingReminderPayload is the payload for EventAdminPendingReminder,
// emitted every 4 hours when items are awaiting admin action.
type AdminPendingReminderPayload struct {
	PendingTransfers   int    `json:"pending_transfers"`
	PendingWithdrawals int    `json:"pending_withdrawals"`
	OldestPendingSince string `json:"oldest_pending_since,omitempty"` // RFC3339 UTC
}

// AdminDailySummaryPayload is the payload for EventAdminDailySummary,
// emitted daily with operational metrics for the preceding 24-hour window.
type AdminDailySummaryPayload struct {
	Date               string `json:"date"` // YYYY-MM-DD (local date)
	NewUsers           int    `json:"new_users"`
	NewTransfers       int    `json:"new_transfers"`
	ApprovedTransfers  int    `json:"approved_transfers"`
	TotalCreditedCents int    `json:"total_credited_cents"`
	NewWithdrawals     int    `json:"new_withdrawals"`
	PendingTransfers   int    `json:"pending_transfers"`
	PendingWithdrawals int    `json:"pending_withdrawals"`
}

// AdminWeeklyReportPayload is the payload for EventAdminWeeklyReport,
// emitted every Monday with revenue and engagement metrics for the past 7 days.
type AdminWeeklyReportPayload struct {
	WeekStartDate     string `json:"week_start_date"` // YYYY-MM-DD
	WeekEndDate       string `json:"week_end_date"`   // YYYY-MM-DD
	TotalRevenueCents int    `json:"total_revenue_cents"`
	NewUsers          int    `json:"new_users"`
	ActiveQuinielas   int    `json:"active_quinielas"`
	TopGroupName      string `json:"top_group_name,omitempty"`
	TopGroupPoints    int    `json:"top_group_points,omitempty"`
	TotalWithdrawals  int    `json:"total_withdrawals"`
	WithdrawalCents   int    `json:"withdrawal_cents"`
}

// KnownEventTypes is the authoritative set of every registered EventType.
// It is derived from eventSamples (event_samples.go) so the two are always
// in sync: a new const without a corresponding eventSamples entry will not
// appear here, making any ValidateTemplate call for that type return an
// "unknown event_type" error — a clear signal that the sample is missing.
var KnownEventTypes = func() map[EventType]struct{} {
	m := make(map[EventType]struct{}, len(eventSamples))
	for k := range eventSamples {
		m[k] = struct{}{}
	}
	return m
}()

// AllEventTypes returns a deterministically sorted slice of every EventType in
// the catalog. The slice is derived from KnownEventTypes (which is derived from
// eventSamples), so a new constant that has no eventSamples entry will not appear
// here — a clear signal that the registration is incomplete.
//
// Use this in exhaustiveness tests to assert that every EventType is handled
// by each dispatch site (priority table, email builder registry, etc.).
func AllEventTypes() []EventType {
	all := make([]EventType, 0, len(KnownEventTypes))
	for et := range KnownEventTypes {
		all = append(all, et)
	}
	sort.Slice(all, func(i, j int) bool { return string(all[i]) < string(all[j]) })
	return all
}
