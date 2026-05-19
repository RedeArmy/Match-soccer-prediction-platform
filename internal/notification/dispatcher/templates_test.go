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
			name:    "WithdrawalStale",
			et:      notification.EventAdminWithdrawalStale,
			payload: notification.AdminWithdrawalPayload{RequestID: 3, UserID: 7, AmountCents: 500, Currency: "GTQ", PendingSince: "2026-01-01T00:00:00Z"},
			want:    "URGENT",
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
