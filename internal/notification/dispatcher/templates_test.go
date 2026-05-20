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
