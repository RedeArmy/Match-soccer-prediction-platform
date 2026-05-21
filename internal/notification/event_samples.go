package notification

import (
	"encoding/json"
	"time"
)

// eventSamples is the authoritative registry of every EventType constant.
// Each entry pairs an EventType with a realistic JSON sample payload used by
// ValidateTemplate to exercise operator templates with missingkey=error.
//
// KnownEventTypes is derived from this map so the two are always in sync:
// adding a new const without an eventSamples entry produces a compile error
// in the KnownEventTypes initialiser (the const would be unreachable from the
// outside but ValidateTemplate would silently return {} for it).
//
// Placement next to the payload structs makes the available template variables
// self-documenting; operators can read the samples to learn what keys their
// templates may reference.
var eventSamples = buildEventSamples()

func buildEventSamples() map[EventType]json.RawMessage {
	adminID := 99
	homeScore, awayScore := 2, 1

	enc := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}

	const (
		sampleQuinielaName = "Mi Quiniela"
		samplePendingSince = "2026-05-21T08:00:00Z"
	)

	deadline := time.Date(2026, 6, 15, 20, 30, 0, 0, time.UTC)
	finished := time.Date(2026, 6, 15, 22, 0, 0, 0, time.UTC)

	return map[EventType]json.RawMessage{
		// ── Prediction ───────────────────────────────────────────────────────────

		EventPredictionConfirmed: enc(PredictionConfirmedPayload{
			UserID: 1, PredictionID: 10, MatchID: 5,
			HomeTeam: "Guatemala", AwayTeam: "Mexico",
		}),
		EventPredictionDeadlineApproach: enc(PredictionDeadlinePayload{
			UserID: 1, MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			DeadlineAt: deadline, MinutesLeft: 30,
		}),
		EventPredictionMissingReminder: enc(PredictionDeadlinePayload{
			UserID: 1, MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			DeadlineAt: deadline, MinutesLeft: 30,
		}),
		EventPredictionLocked: enc(PredictionLockedPayload{
			UserID: 1, MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
		}),
		EventPredictionScored: enc(PredictionScoredPayload{
			UserID: 1, PredictionID: 10, MatchID: 5,
			HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: 2, AwayScore: 1, PointsEarned: 3,
		}),

		// ── Match ────────────────────────────────────────────────────────────────

		EventMatchResultEntered: enc(MatchEventPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: &homeScore, AwayScore: &awayScore,
		}),
		EventMatchPostponed: enc(MatchEventPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: &homeScore, AwayScore: &awayScore,
		}),
		EventMatchCancelled: enc(MatchEventPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			HomeScore: &homeScore, AwayScore: &awayScore,
		}),

		// ── Group ────────────────────────────────────────────────────────────────

		EventGroupJoinRequested: enc(GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}),
		EventGroupJoinApproved: enc(GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}),
		EventGroupJoinRejected: enc(GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}),
		EventGroupMemberJoined: enc(GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}),
		EventGroupMemberLeft: enc(GroupJoinPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, MembershipID: 7,
			UserID: 1, OwnerID: 2,
		}),
		EventGroupLeaderboardMilestone: enc(GroupLeaderboardMilestonePayload{
			UserID: 1, QuinielaID: 3, QuinielaName: sampleQuinielaName,
			NewRank: 2, TotalPoints: 15,
		}),
		EventGroupDisbanded: enc(GroupDisbandedPayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, OwnerID: 2,
		}),
		EventGroupDeadline24h: enc(GroupDeadlinePayload{
			QuinielaID: 3, QuinielaName: sampleQuinielaName, DeadlineAt: deadline,
		}),

		// ── Payment ──────────────────────────────────────────────────────────────

		EventPaymentConfirmed: enc(PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: "REF-001", Reason: "Insufficient funds",
		}),
		EventPaymentFailed: enc(PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: "REF-001", Reason: "Insufficient funds",
		}),
		EventPaymentPendingTimeout: enc(PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: "REF-001", Reason: "Insufficient funds",
		}),
		EventPaymentBankTransferSubmitted: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: "Amount does not match",
		}),
		EventPaymentBankTransferApproved: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: "Amount does not match",
		}),
		EventPaymentBankTransferRejected: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: "Amount does not match",
		}),

		// ── Withdrawal ───────────────────────────────────────────────────────────

		EventWithdrawalRequested: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalApproved: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalRejected: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalProcessing: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalCompleted: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalFailed: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),
		EventWithdrawalPendingTimeout: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: "Daily limit exceeded",
		}),

		// ── Account ──────────────────────────────────────────────────────────────

		EventAccountWelcome: enc(AccountWelcomePayload{
			UserID: 1, UserName: "Juan García", Email: "juan@example.com",
		}),
		EventAccountBalanceCredited: enc(AccountBalancePayload{
			UserID: 1, AmountCents: 5000, BalanceAfter: 15000, Currency: "GTQ",
		}),
		EventAccountBalanceDebited: enc(AccountBalancePayload{
			UserID: 1, AmountCents: 5000, BalanceAfter: 15000, Currency: "GTQ",
		}),
		EventAccountLowBalance: enc(AccountBalancePayload{
			UserID: 1, AmountCents: 5000, BalanceAfter: 15000, Currency: "GTQ",
		}),

		// ── Admin ────────────────────────────────────────────────────────────────

		EventAdminBankTransferPending: enc(AdminBankTransferPayload{
			ProofID: 7, UserID: 1, AmountCents: 12500, Currency: "GTQ",
			QueueDepth: 5, PendingSince: samplePendingSince,
		}),
		EventAdminBankTransferStale: enc(AdminBankTransferPayload{
			ProofID: 7, UserID: 1, AmountCents: 12500, Currency: "GTQ",
			QueueDepth: 5, PendingSince: samplePendingSince,
		}),
		EventAdminBankTransferQueueDepth: enc(AdminBankTransferPayload{
			ProofID: 7, UserID: 1, AmountCents: 12500, Currency: "GTQ",
			QueueDepth: 5, PendingSince: samplePendingSince,
		}),
		EventAdminWithdrawalPending: enc(AdminWithdrawalPayload{
			RequestID: 15, UserID: 1, AmountCents: 50000, Currency: "GTQ",
			PendingSince: samplePendingSince,
		}),
		EventAdminWithdrawalStale: enc(AdminWithdrawalPayload{
			RequestID: 15, UserID: 1, AmountCents: 50000, Currency: "GTQ",
			PendingSince: samplePendingSince,
		}),
		EventAdminHighValueWithdrawal: enc(AdminWithdrawalPayload{
			RequestID: 15, UserID: 1, AmountCents: 50000, Currency: "GTQ",
			PendingSince: samplePendingSince,
		}),
		EventAdminMatchResultPending: enc(AdminMatchResultPayload{
			MatchID: 5, HomeTeam: "Guatemala", AwayTeam: "Mexico",
			FinishedAt: finished, MinutesElapsed: 90,
		}),
		EventAdminPendingReminder: enc(AdminPendingReminderPayload{
			PendingTransfers: 3, PendingWithdrawals: 2,
			OldestPendingSince: samplePendingSince,
		}),
		EventAdminDailySummary: enc(AdminDailySummaryPayload{
			Date: "2026-05-21", NewUsers: 5, NewTransfers: 10, ApprovedTransfers: 8,
			TotalCreditedCents: 125000, NewWithdrawals: 3,
			PendingTransfers: 2, PendingWithdrawals: 1,
		}),
		EventAdminWeeklyReport: enc(AdminWeeklyReportPayload{
			WeekStartDate: "2026-05-15", WeekEndDate: "2026-05-21",
			TotalRevenueCents: 500000, NewUsers: 25, ActiveQuinielas: 12,
			TopGroupName: "Los Campeones", TopGroupPoints: 42,
			TotalWithdrawals: 8, WithdrawalCents: 200000,
		}),

		// ── Admin alerts (shared payload shape) ──────────────────────────────────

		EventAdminPaymentDispute: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventAdminScoringDiscrepancy: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventAdminGroupReported: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),

		// ── System / infrastructure alerts ───────────────────────────────────────

		EventSystemCircuitBreakerOpened: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemCircuitBreakerHalfOpen: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemWebhookSignatureFailed: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemWebhookSignatureRepeated: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemTxRetryExhausted: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemBalanceLedgerMismatch: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemRateLimitAbuse: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemIdempotencyCollision: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
		EventSystemFileStoreUnavailable: enc(SystemAlertPayload{
			Component: "payment-service", Detail: "timeout after 3 retries", Severity: "critical",
		}),
	}
}

// SamplePayload returns a realistic JSON payload for the given event type,
// suitable for template validation with missingkey=error.  Returns an empty
// JSON object for unknown event types (callers should treat that as a
// misconfiguration, not a valid sample).
func SamplePayload(et EventType) json.RawMessage {
	if b, ok := eventSamples[et]; ok {
		return b
	}
	return json.RawMessage(`{}`)
}
