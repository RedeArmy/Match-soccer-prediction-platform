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
		sampleQuinielaName      = "Mi Quiniela"
		samplePendingSince      = "2026-05-21T08:00:00Z"
		sampleInsufficientFunds = "Insufficient funds"
		samplePaymentRef        = "REF-001"
		sampleAmountMismatch    = "Amount does not match"
		sampleDailyLimit        = "Daily limit exceeded"
		sampleAlertComponent    = "payment-service"
		sampleAlertDetail       = "timeout after 3 retries"
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
			Currency: "GTQ", Reference: samplePaymentRef, Reason: sampleInsufficientFunds,
		}),
		EventPaymentFailed: enc(PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: samplePaymentRef, Reason: sampleInsufficientFunds,
		}),
		EventPaymentPendingTimeout: enc(PaymentPayload{
			UserID: 1, PaymentID: 42, AmountCents: 12500,
			Currency: "GTQ", Reference: samplePaymentRef, Reason: sampleInsufficientFunds,
		}),
		EventPaymentBankTransferSubmitted: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleAmountMismatch,
		}),
		EventPaymentBankTransferApproved: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleAmountMismatch,
		}),
		EventPaymentBankTransferRejected: enc(BankTransferPayload{
			UserID: 1, ProofID: 7, AmountCents: 12500,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleAmountMismatch,
		}),

		// ── Withdrawal ───────────────────────────────────────────────────────────

		EventWithdrawalRequested: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalApproved: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalRejected: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalProcessing: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalCompleted: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalFailed: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
		}),
		EventWithdrawalPendingTimeout: enc(WithdrawalPayload{
			UserID: 1, RequestID: 15, AmountCents: 50000,
			Currency: "GTQ", AdminID: &adminID, Notes: sampleDailyLimit,
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
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventAdminScoringDiscrepancy: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventAdminGroupReported: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),

		// ── System / infrastructure alerts ───────────────────────────────────────

		EventSystemCircuitBreakerOpened: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemCircuitBreakerHalfOpen: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemWebhookSignatureFailed: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemWebhookSignatureRepeated: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemTxRetryExhausted: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemBalanceLedgerMismatch: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemRateLimitAbuse: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemIdempotencyCollision: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),
		EventSystemFileStoreUnavailable: enc(SystemAlertPayload{
			Component: sampleAlertComponent, Detail: sampleAlertDetail, Severity: "critical",
		}),

		// ── KYC compliance events ─────────────────────────────────────────────────
		EventKYCWinnerFreeze: enc(KYCWinnerFreezePayload{
			UserID: 1, AmountCents: 50_000, TraceID: "00-abc123-def456-01",
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
