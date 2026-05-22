package notification

// Priority classifies the urgency of a notification event and determines
// which delivery channels are activated by the dispatcher.
//
//   - P0 Critical: money or security at risk; admin email is mandatory and
//     immediate; user channels are best-effort alongside it.
//   - P1 High: action required or direct user impact; push + in-app; email
//     for terminal payment / withdrawal states.
//   - P2 Medium: informative but experience-relevant; in-app always; push
//     for match events and group milestones.
//   - P3 Low: summaries and low-signal updates; in-app only or batched digest.
type Priority int

// Priority levels ordered from most to least urgent.
const (
	PriorityP0Critical Priority = 0
	PriorityP1High     Priority = 1
	PriorityP2Medium   Priority = 2
	PriorityP3Low      Priority = 3
)

// priorityTable maps every known EventType to its delivery priority.
// Events absent from the table default to PriorityP2Medium.
var priorityTable = map[EventType]Priority{
	// P0 — Critical
	EventPaymentBankTransferApproved:    PriorityP0Critical,
	EventPaymentBankTransferRejected:    PriorityP0Critical,
	EventWithdrawalApproved:             PriorityP0Critical,
	EventWithdrawalRejected:             PriorityP0Critical,
	EventWithdrawalCompleted:            PriorityP0Critical,
	EventWithdrawalFailed:               PriorityP0Critical,
	EventAdminBankTransferPending:       PriorityP0Critical,
	EventAdminBankTransferStale:         PriorityP0Critical,
	EventAdminBankTransferQueueDepth:    PriorityP0Critical,
	EventAdminWithdrawalPending:         PriorityP0Critical,
	EventAdminWithdrawalStale:           PriorityP0Critical,
	EventAdminHighValueWithdrawal:       PriorityP0Critical,
	EventAdminPaymentDispute:            PriorityP0Critical,
	EventAdminScoringDiscrepancy:        PriorityP0Critical,
	EventSystemWebhookSignatureFailed:   PriorityP0Critical,
	EventSystemWebhookSignatureRepeated: PriorityP0Critical,
	EventSystemBalanceLedgerMismatch:    PriorityP0Critical,
	EventSystemCircuitBreakerOpened:     PriorityP0Critical,

	// P1 — High
	EventPredictionDeadlineApproach: PriorityP1High,
	EventPredictionMissingReminder:  PriorityP1High,
	EventPredictionScored:           PriorityP1High,
	EventMatchResultEntered:         PriorityP1High,
	EventMatchPostponed:             PriorityP1High,
	EventMatchCancelled:             PriorityP1High,
	EventGroupJoinApproved:          PriorityP1High,
	EventGroupJoinRejected:          PriorityP1High,
	EventGroupDisbanded:             PriorityP1High,
	EventGroupDeadline24h:           PriorityP1High,
	EventPaymentConfirmed:           PriorityP1High,
	EventPaymentFailed:              PriorityP1High,
	EventPaymentPendingTimeout:      PriorityP1High,
	EventWithdrawalPendingTimeout:   PriorityP1High,
	EventAccountLowBalance:          PriorityP1High,
	EventAdminMatchResultPending:    PriorityP1High,
	EventAdminGroupReported:         PriorityP1High,
	EventAdminPendingReminder:       PriorityP1High,
	EventSystemTxRetryExhausted:     PriorityP1High,
	EventSystemFileStoreUnavailable: PriorityP1High,

	// P2 — Medium (default, listed for completeness)
	EventPredictionConfirmed:          PriorityP2Medium,
	EventPredictionLocked:             PriorityP2Medium,
	EventGroupJoinRequested:           PriorityP2Medium,
	EventGroupLeaderboardMilestone:    PriorityP2Medium,
	EventPaymentBankTransferSubmitted: PriorityP2Medium,
	EventWithdrawalRequested:          PriorityP2Medium,
	EventWithdrawalProcessing:         PriorityP2Medium,
	EventAccountWelcome:               PriorityP2Medium,
	EventAccountBalanceCredited:       PriorityP2Medium,
	EventAdminDailySummary:            PriorityP2Medium,
	EventAdminWeeklyReport:            PriorityP2Medium,
	EventSystemCircuitBreakerHalfOpen: PriorityP2Medium,
	EventSystemRateLimitAbuse:         PriorityP2Medium,
	EventSystemIdempotencyCollision:   PriorityP2Medium,

	// P3 — Low
	EventGroupMemberJoined:     PriorityP3Low,
	EventGroupMemberLeft:       PriorityP3Low,
	EventAccountBalanceDebited: PriorityP3Low,
}

// PriorityOf returns the Priority for the given EventType.
// Unknown event types default to PriorityP2Medium.
func PriorityOf(et EventType) Priority {
	if p, ok := priorityTable[et]; ok {
		return p
	}
	return PriorityP2Medium
}

// adminEventSet is the authoritative set of EventTypes directed at administrators.
// Adding a new admin/system event type here is the single update point — no switch
// arms to forget. IsAdminEvent is O(1) and CC=2 (consistent with priorityTable).
var adminEventSet = map[EventType]struct{}{
	EventAdminBankTransferPending:       {},
	EventAdminBankTransferStale:         {},
	EventAdminBankTransferQueueDepth:    {},
	EventAdminWithdrawalPending:         {},
	EventAdminWithdrawalStale:           {},
	EventAdminHighValueWithdrawal:       {},
	EventAdminPaymentDispute:            {},
	EventAdminMatchResultPending:        {},
	EventAdminScoringDiscrepancy:        {},
	EventAdminGroupReported:             {},
	EventAdminPendingReminder:           {},
	EventAdminDailySummary:              {},
	EventAdminWeeklyReport:              {},
	EventSystemCircuitBreakerOpened:     {},
	EventSystemCircuitBreakerHalfOpen:   {},
	EventSystemWebhookSignatureFailed:   {},
	EventSystemWebhookSignatureRepeated: {},
	EventSystemTxRetryExhausted:         {},
	EventSystemBalanceLedgerMismatch:    {},
	EventSystemRateLimitAbuse:           {},
	EventSystemIdempotencyCollision:     {},
	EventSystemFileStoreUnavailable:     {},
}

// IsAdminEvent reports whether the event type is directed at administrators
// rather than (or in addition to) the originating user. Admin events always
// trigger an immediate email to all active admin recipients.
func IsAdminEvent(et EventType) bool {
	_, ok := adminEventSet[et]
	return ok
}
