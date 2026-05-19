package notification_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// TestPriorityOf_P0Events verifies that all P0-critical events are classified
// correctly.
func TestPriorityOf_P0Events(t *testing.T) {
	t.Parallel()

	p0Events := []notification.EventType{
		notification.EventPaymentBankTransferApproved,
		notification.EventPaymentBankTransferRejected,
		notification.EventWithdrawalApproved,
		notification.EventWithdrawalRejected,
		notification.EventWithdrawalCompleted,
		notification.EventWithdrawalFailed,
		notification.EventAdminBankTransferPending,
		notification.EventAdminBankTransferStale,
		notification.EventAdminBankTransferQueueDepth,
		notification.EventAdminWithdrawalPending,
		notification.EventAdminWithdrawalStale,
		notification.EventAdminHighValueWithdrawal,
		notification.EventAdminPaymentDispute,
		notification.EventAdminScoringDiscrepancy,
		notification.EventSystemWebhookSignatureFailed,
		notification.EventSystemWebhookSignatureRepeated,
		notification.EventSystemBalanceLedgerMismatch,
		notification.EventSystemCircuitBreakerOpened,
	}

	for _, et := range p0Events {
		if got := notification.PriorityOf(et); got != notification.PriorityP0Critical {
			t.Errorf("PriorityOf(%q) = %d; want P0 (%d)", et, got, notification.PriorityP0Critical)
		}
	}
}

// TestPriorityOf_P1Events verifies a representative sample of P1 events.
func TestPriorityOf_P1Events(t *testing.T) {
	t.Parallel()

	p1Events := []notification.EventType{
		notification.EventPredictionDeadlineApproach,
		notification.EventPredictionScored,
		notification.EventMatchPostponed,
		notification.EventGroupJoinApproved,
		notification.EventGroupDisbanded,
		notification.EventPaymentConfirmed,
		notification.EventPaymentFailed,
		notification.EventAccountLowBalance,
	}

	for _, et := range p1Events {
		if got := notification.PriorityOf(et); got != notification.PriorityP1High {
			t.Errorf("PriorityOf(%q) = %d; want P1 (%d)", et, got, notification.PriorityP1High)
		}
	}
}

// TestPriorityOf_DefaultMedium verifies that an unknown event type defaults to
// PriorityP2Medium.
func TestPriorityOf_DefaultMedium(t *testing.T) {
	t.Parallel()

	unknown := notification.EventType("unknown.event.type.xyz")
	if got := notification.PriorityOf(unknown); got != notification.PriorityP2Medium {
		t.Errorf("PriorityOf(unknown) = %d; want P2 (%d)", got, notification.PriorityP2Medium)
	}
}

// TestIsAdminEvent verifies that admin and system events are classified as
// admin events and that user-facing events are not.
func TestIsAdminEvent(t *testing.T) {
	t.Parallel()

	adminEvents := []notification.EventType{
		notification.EventAdminBankTransferPending,
		notification.EventAdminBankTransferStale,
		notification.EventAdminBankTransferQueueDepth,
		notification.EventAdminWithdrawalPending,
		notification.EventAdminWithdrawalStale,
		notification.EventAdminHighValueWithdrawal,
		notification.EventAdminPaymentDispute,
		notification.EventAdminMatchResultPending,
		notification.EventAdminScoringDiscrepancy,
		notification.EventAdminGroupReported,
		notification.EventSystemCircuitBreakerOpened,
		notification.EventSystemCircuitBreakerHalfOpen,
		notification.EventSystemWebhookSignatureFailed,
		notification.EventSystemWebhookSignatureRepeated,
		notification.EventSystemTxRetryExhausted,
		notification.EventSystemBalanceLedgerMismatch,
		notification.EventSystemRateLimitAbuse,
		notification.EventSystemIdempotencyCollision,
		notification.EventSystemFileStoreUnavailable,
	}

	userEvents := []notification.EventType{
		notification.EventPredictionConfirmed,
		notification.EventPredictionScored,
		notification.EventPaymentConfirmed,
		notification.EventWithdrawalCompleted,
		notification.EventGroupJoinApproved,
		notification.EventAccountWelcome,
	}

	for _, et := range adminEvents {
		if !notification.IsAdminEvent(et) {
			t.Errorf("IsAdminEvent(%q) = false; want true", et)
		}
	}
	for _, et := range userEvents {
		if notification.IsAdminEvent(et) {
			t.Errorf("IsAdminEvent(%q) = true; want false", et)
		}
	}
}

// TestPriorityConstants_Order verifies the numeric ordering of priority
// constants (P0 < P1 < P2 < P3).
func TestPriorityConstants_Order(t *testing.T) {
	t.Parallel()

	if notification.PriorityP0Critical >= notification.PriorityP1High {
		t.Error("P0 must be less than P1")
	}
	if notification.PriorityP1High >= notification.PriorityP2Medium {
		t.Error("P1 must be less than P2")
	}
	if notification.PriorityP2Medium >= notification.PriorityP3Low {
		t.Error("P2 must be less than P3")
	}
}
