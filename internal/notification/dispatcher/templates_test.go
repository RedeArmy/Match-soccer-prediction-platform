package dispatcher

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// TestBuildEmailData_AllEventTypes exercises every switch branch in
// buildEmailData, ensuring no case panics and each produces a non-empty subject
// that contains the expected keyword.
func TestBuildEmailData_AllEventTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		et      notification.EventType
		payload any
		want    string // expected substring in subject
	}{
		{
			name:    "BankTransferPending",
			et:      notification.EventAdminBankTransferPending,
			payload: notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 5000, Currency: "GTQ"},
			want:    "ACTION REQUIRED",
		},
		{
			name:    "BankTransferStale",
			et:      notification.EventAdminBankTransferStale,
			payload: notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 0, Currency: "GTQ", PendingSince: "2026-01-01T00:00:00Z"},
			want:    "URGENT",
		},
		{
			name:    "BankTransferQueueDepth",
			et:      notification.EventAdminBankTransferQueueDepth,
			payload: notification.AdminBankTransferPayload{QueueDepth: 10},
			want:    "queue",
		},
		{
			name:    "WithdrawalPending",
			et:      notification.EventAdminWithdrawalPending,
			payload: notification.AdminWithdrawalPayload{RequestID: 5, UserID: 9, AmountCents: 10000, Currency: "GTQ"},
			want:    "ACTION REQUIRED",
		},
		{
			name:    "WithdrawalStale",
			et:      notification.EventAdminWithdrawalStale,
			payload: notification.AdminWithdrawalPayload{RequestID: 3, UserID: 7, AmountCents: 500, Currency: "GTQ", PendingSince: "2026-01-01T00:00:00Z"},
			want:    "URGENT",
		},
		{
			name:    "HighValueWithdrawal",
			et:      notification.EventAdminHighValueWithdrawal,
			payload: notification.AdminWithdrawalPayload{RequestID: 6, UserID: 3, AmountCents: 500000, Currency: "GTQ"},
			want:    "CRITICAL",
		},
		{
			name:    "CircuitBreakerOpened",
			et:      notification.EventSystemCircuitBreakerOpened,
			payload: notification.SystemAlertPayload{Component: "email", Detail: "5 failures", Severity: "critical"},
			want:    "CRITICAL",
		},
		{
			name:    "BalanceLedgerMismatch",
			et:      notification.EventSystemBalanceLedgerMismatch,
			payload: notification.SystemAlertPayload{Component: "ledger", Detail: "mismatch detected", Severity: "critical"},
			want:    "CRITICAL",
		},
		{
			name:    "WebhookSignatureFailed",
			et:      notification.EventSystemWebhookSignatureFailed,
			payload: notification.SystemAlertPayload{Component: "recurrente", Detail: "bad sig", Severity: "high"},
			want:    "SECURITY",
		},
		{
			name:    "WebhookSignatureRepeated",
			et:      notification.EventSystemWebhookSignatureRepeated,
			payload: notification.SystemAlertPayload{Component: "paypal", Detail: "repeated", Severity: "high"},
			want:    "SECURITY",
		},
		{
			name:    "PaymentDispute",
			et:      notification.EventAdminPaymentDispute,
			payload: notification.SystemAlertPayload{Component: "payment", Detail: "dispute filed"},
			want:    "ACTION REQUIRED",
		},
		{
			name:    "PendingReminder",
			et:      notification.EventAdminPendingReminder,
			payload: notification.AdminPendingReminderPayload{PendingTransfers: 2, PendingWithdrawals: 1, OldestPendingSince: "2026-05-01T10:00:00Z"},
			want:    "ACTION REQUIRED",
		},
		{
			name: "PendingReminderNoOldest",
			et:   notification.EventAdminPendingReminder,
			payload: notification.AdminPendingReminderPayload{
				PendingTransfers:   1,
				PendingWithdrawals: 0,
				OldestPendingSince: "",
			},
			want: "ACTION REQUIRED",
		},
		{
			name: "DailySummary",
			et:   notification.EventAdminDailySummary,
			payload: notification.AdminDailySummaryPayload{
				Date: "2026-05-19", NewUsers: 3, NewTransfers: 5,
				ApprovedTransfers: 4, TotalCreditedCents: 50000,
				NewWithdrawals: 2, PendingTransfers: 1, PendingWithdrawals: 0,
			},
			want: "DAILY SUMMARY",
		},
		{
			name: "WeeklyReportWithTopGroup",
			et:   notification.EventAdminWeeklyReport,
			payload: notification.AdminWeeklyReportPayload{
				WeekStartDate: "2026-05-13", WeekEndDate: "2026-05-19",
				TotalRevenueCents: 200000, NewUsers: 10, ActiveQuinielas: 7,
				TopGroupName: "Oficina A", TopGroupPoints: 42,
				TotalWithdrawals: 3, WithdrawalCents: 30000,
			},
			want: "WEEKLY REPORT",
		},
		{
			name: "WeeklyReportNoTopGroup",
			et:   notification.EventAdminWeeklyReport,
			payload: notification.AdminWeeklyReportPayload{
				WeekStartDate: "2026-05-13", WeekEndDate: "2026-05-19",
				TotalRevenueCents: 0, NewUsers: 0, ActiveQuinielas: 0,
			},
			want: "WEEKLY REPORT",
		},
		// ── Match / Scoring ───────────────────────────────────────────────────
		{
			name: "MatchResultPending",
			et:   notification.EventAdminMatchResultPending,
			payload: notification.AdminMatchResultPayload{
				MatchID: 7, HomeTeam: "Brasil", AwayTeam: "Francia",
				MinutesElapsed: 97,
			},
			want: "ACTION REQUIRED",
		},
		{
			name:    "ScoringDiscrepancy",
			et:      notification.EventAdminScoringDiscrepancy,
			payload: notification.SystemAlertPayload{Component: "scorer", Detail: "delta -3pts", Severity: "critical"},
			want:    "CRITICAL",
		},
		{
			name:    "GroupReported",
			et:      notification.EventAdminGroupReported,
			payload: notification.SystemAlertPayload{Component: "group:42", Detail: "spam content"},
			want:    "ACTION REQUIRED",
		},
		// ── System alerts ─────────────────────────────────────────────────────
		{
			name:    "CircuitBreakerHalfOpen",
			et:      notification.EventSystemCircuitBreakerHalfOpen,
			payload: notification.SystemAlertPayload{Component: "email", Detail: "1 probe sent", Severity: "medium"},
			want:    "WARNING",
		},
		{
			name:    "TxRetryExhaustedNoAffectedIDs",
			et:      notification.EventSystemTxRetryExhausted,
			payload: notification.SystemAlertPayload{Component: "payment_service", Detail: "5 retries", Severity: "critical"},
			want:    "CRITICAL",
		},
		{
			name:    "TxRetryExhaustedWithAffectedIDs",
			et:      notification.EventSystemTxRetryExhausted,
			payload: notification.SystemAlertPayload{Component: "payment_service", Detail: "5 retries", Severity: "critical", AffectedIDs: []int{101, 202}},
			want:    "CRITICAL",
		},
		{
			name:    "RateLimitAbuse",
			et:      notification.EventSystemRateLimitAbuse,
			payload: notification.SystemAlertPayload{Component: "api_gateway", Detail: "2000 req/min from 1.2.3.4"},
			want:    "SECURITY",
		},
		{
			name:    "IdempotencyCollision",
			et:      notification.EventSystemIdempotencyCollision,
			payload: notification.SystemAlertPayload{Component: "withdrawal_service", Detail: "key=abc payload mismatch"},
			want:    "WARNING",
		},
		{
			name:    "FileStoreUnavailable",
			et:      notification.EventSystemFileStoreUnavailable,
			payload: notification.SystemAlertPayload{Component: "s3", Detail: "connection timeout", Severity: "critical"},
			want:    "CRITICAL",
		},
		{
			name:    "UnknownEvent",
			et:      notification.EventType("unknown.custom.event"),
			payload: nil,
			want:    "ADMIN ALERT",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entry, err := notification.NewOutboxEntry(tc.et, "test", "1", tc.payload)
			if err != nil {
				t.Fatalf("NewOutboxEntry: %v", err)
			}

			subject, html, renderErr := renderEmail(entry)
			if renderErr != nil {
				t.Fatalf("renderEmail: %v", renderErr)
			}
			if !strings.Contains(subject, tc.want) {
				t.Errorf("subject %q does not contain %q", subject, tc.want)
			}
			if len(html) == 0 {
				t.Error("html body is empty")
			}
		})
	}
}

// TestBuildEmailData_DetailsOrdering_Deterministic verifies that the detail
// table rows are rendered in the same order on every invocation, which is
// guaranteed because Details is a []emailDetail (not a map).
func TestBuildEmailData_DetailsOrdering_Deterministic(t *testing.T) {
	t.Parallel()

	entry, err := notification.NewOutboxEntry(
		notification.EventAdminBankTransferPending,
		"test", "1",
		notification.AdminBankTransferPayload{ProofID: 42, UserID: 7, AmountCents: 5000, Currency: "GTQ"},
	)
	if err != nil {
		t.Fatalf("NewOutboxEntry: %v", err)
	}

	_, html1, _ := renderEmail(entry)
	_, html2, _ := renderEmail(entry)
	_, html3, _ := renderEmail(entry)

	if html1 != html2 || html2 != html3 {
		t.Error("renderEmail output is not deterministic across invocations")
	}

	// Proof ID must appear before User ID, which must appear before Amount.
	if p, u, a := strings.Index(html1, "Proof ID"), strings.Index(html1, "User ID"), strings.Index(html1, "Amount"); p >= u || u >= a {
		t.Errorf("detail rows out of expected order: Proof ID@%d UserID@%d Amount@%d", p, u, a)
	}
}

// TestBuildUserContent_ActionURL_PopulatedForKnownEvents verifies that every
// known event type produces a non-empty, correctly routed action URL and that
// the default branch (unknown event) does not.
func TestBuildUserContent_ActionURL_PopulatedForKnownEvents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		et      notification.EventType
		payload any
		want    string
	}{
		{notification.EventPredictionConfirmed, notification.PredictionConfirmedPayload{UserID: 1, MatchID: 3}, "/api/v1/predictions/me"},
		{notification.EventPredictionDeadlineApproach, notification.PredictionDeadlinePayload{UserID: 1, MatchID: 7}, "/api/v1/matches/7"},
		{notification.EventPredictionMissingReminder, notification.PredictionDeadlinePayload{UserID: 1, MatchID: 8}, "/api/v1/matches/8"},
		{notification.EventPredictionLocked, notification.PredictionLockedPayload{UserID: 1, MatchID: 9}, "/api/v1/matches/9"},
		{notification.EventPredictionScored, notification.PredictionScoredPayload{UserID: 1, MatchID: 2}, "/api/v1/predictions/me"},
		{notification.EventMatchResultEntered, notification.MatchEventPayload{MatchID: 11}, "/api/v1/matches/11"},
		{notification.EventMatchPostponed, notification.MatchEventPayload{MatchID: 12}, "/api/v1/matches/12"},
		{notification.EventMatchCancelled, notification.MatchEventPayload{MatchID: 13}, "/api/v1/matches/13"},
		{notification.EventGroupJoinApproved, notification.GroupJoinPayload{UserID: 1, QuinielaID: 5}, "/api/v1/groups/5"},
		{notification.EventGroupJoinRejected, notification.GroupJoinPayload{UserID: 1, QuinielaID: 6}, "/api/v1/groups/me"},
		{notification.EventGroupDisbanded, notification.GroupDisbandedPayload{QuinielaID: 7}, "/api/v1/groups/me"},
		{notification.EventGroupDeadline24h, notification.GroupDeadlinePayload{QuinielaID: 8}, "/api/v1/groups/8"},
		{notification.EventGroupLeaderboardMilestone, notification.GroupLeaderboardMilestonePayload{UserID: 1, QuinielaID: 9}, "/api/v1/groups/9/leaderboard"},
		{notification.EventGroupMemberJoined, notification.GroupJoinPayload{UserID: 2, QuinielaID: 10}, "/api/v1/groups/10/members"},
		{notification.EventGroupMemberLeft, notification.GroupJoinPayload{UserID: 3, QuinielaID: 10}, "/api/v1/groups/10/members"},
		{notification.EventPaymentConfirmed, notification.PaymentPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		{notification.EventPaymentFailed, notification.PaymentPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/payment-intents"},
		{notification.EventPaymentBankTransferSubmitted, notification.BankTransferPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/bank-transfers"},
		{notification.EventPaymentBankTransferApproved, notification.BankTransferPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		{notification.EventPaymentBankTransferRejected, notification.BankTransferPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/bank-transfers"},
		{notification.EventPaymentPendingTimeout, notification.PaymentPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/payment-intents"},
		{notification.EventWithdrawalRequested, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventWithdrawalApproved, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventWithdrawalRejected, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventWithdrawalProcessing, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventWithdrawalCompleted, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		{notification.EventWithdrawalFailed, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventWithdrawalPendingTimeout, notification.WithdrawalPayload{UserID: 1, AmountCents: 100, Currency: "GTQ"}, "/api/v1/withdrawals"},
		{notification.EventAccountWelcome, notification.AccountWelcomePayload{UserID: 1, UserName: "Alice"}, "/api/v1/groups/me"},
		{notification.EventAccountBalanceCredited, notification.AccountBalancePayload{UserID: 1, AmountCents: 100, BalanceAfter: 500, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		{notification.EventAccountBalanceDebited, notification.AccountBalancePayload{UserID: 1, AmountCents: 100, BalanceAfter: 400, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		{notification.EventAccountLowBalance, notification.AccountBalancePayload{UserID: 1, BalanceAfter: 50, Currency: "GTQ"}, "/api/v1/users/me/balance"},
		// Default branch: unknown event must produce an empty actionURL.
		{notification.EventType("custom.unknown"), notification.PredictionConfirmedPayload{UserID: 1}, ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.et), func(t *testing.T) {
			t.Parallel()
			entry, err := notification.NewOutboxEntry(tc.et, "test", "1", tc.payload)
			if err != nil {
				t.Fatalf("NewOutboxEntry: %v", err)
			}
			got := buildUserContent(entry, LocaleEN)
			if got.actionURL != tc.want {
				t.Errorf("actionURL = %q; want %q", got.actionURL, tc.want)
			}
		})
	}
}

func TestFormatCents_Zero(t *testing.T) {
	t.Parallel()

	got := formatCents(0, "GTQ")
	if got != "0.00 GTQ" {
		t.Errorf("formatCents(0, GTQ) = %q; want %q", got, "0.00 GTQ")
	}
}

func TestFormatCents_NonZero(t *testing.T) {
	t.Parallel()

	got := formatCents(150050, "USD")
	if got != "1500.50 USD" {
		t.Errorf("formatCents(150050, USD) = %q; want %q", got, "1500.50 USD")
	}
}
