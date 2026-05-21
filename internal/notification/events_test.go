package notification_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

// TestSamplePayload_KnownEventType_ReturnsNonNil verifies that every known
// event type has a non-nil sample payload registered in the event catalog.
func TestSamplePayload_KnownEventType_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	for et := range notification.KnownEventTypes {
		if got := notification.SamplePayload(et); got == nil {
			t.Errorf("SamplePayload(%q) = nil; every known event type must have a sample", et)
		}
	}
}

// TestSamplePayload_UnknownEventType_ReturnsEmptyObject verifies that an
// unregistered event type returns an empty JSON object rather than panicking.
func TestSamplePayload_UnknownEventType_ReturnsEmptyObject(t *testing.T) {
	t.Parallel()

	got := notification.SamplePayload("totally.unknown.event.type")
	if string(got) != "{}" {
		t.Errorf("SamplePayload(unknown) = %q; want %q", string(got), "{}")
	}
}

// TestEventType_StringValues verifies that every exported EventType constant
// is a non-empty string and that its string representation matches the value.
func TestEventType_StringValues(t *testing.T) {
	t.Parallel()

	types := []notification.EventType{
		// Prediction
		notification.EventPredictionConfirmed,
		notification.EventPredictionDeadlineApproach,
		notification.EventPredictionMissingReminder,
		notification.EventPredictionLocked,
		notification.EventPredictionScored,
		// Match
		notification.EventMatchResultEntered,
		notification.EventMatchPostponed,
		notification.EventMatchCancelled,
		// Group
		notification.EventGroupJoinRequested,
		notification.EventGroupJoinApproved,
		notification.EventGroupJoinRejected,
		notification.EventGroupMemberJoined,
		notification.EventGroupMemberLeft,
		notification.EventGroupLeaderboardMilestone,
		notification.EventGroupDisbanded,
		notification.EventGroupDeadline24h,
		// Payment
		notification.EventPaymentConfirmed,
		notification.EventPaymentFailed,
		notification.EventPaymentBankTransferSubmitted,
		notification.EventPaymentBankTransferApproved,
		notification.EventPaymentBankTransferRejected,
		notification.EventPaymentPendingTimeout,
		// Withdrawal
		notification.EventWithdrawalRequested,
		notification.EventWithdrawalApproved,
		notification.EventWithdrawalRejected,
		notification.EventWithdrawalProcessing,
		notification.EventWithdrawalCompleted,
		notification.EventWithdrawalFailed,
		notification.EventWithdrawalPendingTimeout,
		// Account
		notification.EventAccountWelcome,
		notification.EventAccountBalanceCredited,
		notification.EventAccountBalanceDebited,
		notification.EventAccountLowBalance,
		// Admin
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
		// System
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

	seen := make(map[notification.EventType]bool)
	for _, et := range types {
		if et == "" {
			t.Errorf("empty EventType constant detected")
			continue
		}
		if seen[et] {
			t.Errorf("duplicate EventType value: %q", et)
		}
		seen[et] = true
	}
}

// TestNewOutboxEntry_HappyPath verifies that NewOutboxEntry marshals the
// payload and populates all required fields.
func TestNewOutboxEntry_HappyPath(t *testing.T) {
	t.Parallel()

	payload := notification.BankTransferPayload{
		UserID:      5,
		ProofID:     100,
		AmountCents: 50000,
		Currency:    "GTQ",
	}

	entry, err := notification.NewOutboxEntry(
		notification.EventAdminBankTransferPending,
		"bank_transfer_proof", "100",
		payload,
	)
	if err != nil {
		t.Fatalf("NewOutboxEntry: %v", err)
	}

	if entry.EventType != notification.EventAdminBankTransferPending {
		t.Errorf("EventType: got %q; want %q", entry.EventType, notification.EventAdminBankTransferPending)
	}
	if entry.AggregateType != "bank_transfer_proof" {
		t.Errorf("AggregateType: got %q; want bank_transfer_proof", entry.AggregateType)
	}
	if entry.AggregateID != "100" {
		t.Errorf("AggregateID: got %q; want 100", entry.AggregateID)
	}
	if len(entry.Payload) == 0 {
		t.Error("Payload is empty")
	}
	if entry.Status != notification.OutboxStatusPending {
		t.Errorf("Status: got %q; want pending", entry.Status)
	}
	if entry.MaxAttempts != 5 {
		t.Errorf("MaxAttempts: got %d; want 5", entry.MaxAttempts)
	}
	if entry.ScheduledAt.IsZero() {
		t.Error("ScheduledAt must not be zero")
	}
}

// TestNewOutboxEntry_InvalidPayload verifies that a non-marshallable payload
// returns an error.
func TestNewOutboxEntry_InvalidPayload(t *testing.T) {
	t.Parallel()

	_, err := notification.NewOutboxEntry(
		notification.EventSystemCircuitBreakerOpened,
		"system", "1",
		make(chan int), // channels are not JSON-marshallable
	)
	if err == nil {
		t.Fatal("expected error for non-marshallable payload; got nil")
	}
}

// TestOutboxEntry_DecodePayload verifies the round-trip through DecodePayload.
func TestOutboxEntry_DecodePayload(t *testing.T) {
	t.Parallel()

	original := notification.WithdrawalPayload{
		UserID:      42,
		RequestID:   99,
		AmountCents: 75000,
		Currency:    "GTQ",
	}

	entry, err := notification.NewOutboxEntry(
		notification.EventWithdrawalApproved,
		"withdrawal_request", "99",
		original,
	)
	if err != nil {
		t.Fatalf("NewOutboxEntry: %v", err)
	}

	var decoded notification.WithdrawalPayload
	if err := entry.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if decoded.UserID != original.UserID {
		t.Errorf("UserID: got %d; want %d", decoded.UserID, original.UserID)
	}
	if decoded.AmountCents != original.AmountCents {
		t.Errorf("AmountCents: got %d; want %d", decoded.AmountCents, original.AmountCents)
	}
}

// TestOutboxEntry_DecodePayload_BadJSON verifies that DecodePayload returns an
// error when the stored JSON is invalid for the target type.
func TestOutboxEntry_DecodePayload_BadJSON(t *testing.T) {
	t.Parallel()

	entry := &notification.OutboxEntry{
		Payload: json.RawMessage(`{not valid json`),
	}
	var dst notification.WithdrawalPayload
	if err := entry.DecodePayload(&dst); err == nil {
		t.Fatal("expected error for invalid JSON; got nil")
	}
}

// TestOutboxStatusConstants verifies all OutboxStatus values are non-empty strings.
func TestOutboxStatusConstants(t *testing.T) {
	t.Parallel()

	statuses := []notification.OutboxStatus{
		notification.OutboxStatusPending,
		notification.OutboxStatusProcessing,
		notification.OutboxStatusDone,
		notification.OutboxStatusFailed,
	}
	seen := make(map[notification.OutboxStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("empty OutboxStatus constant")
		}
		if seen[s] {
			t.Errorf("duplicate OutboxStatus: %q", s)
		}
		seen[s] = true
	}
}

// TestPayloadStructs_JSONRoundtrip spot-checks several payload structs to
// ensure they marshal and unmarshal without data loss.
func TestPayloadStructs_JSONRoundtrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload any
		target  func() any
		check   func(t *testing.T, decoded any)
	}{
		{
			name: "PredictionScoredPayload",
			payload: notification.PredictionScoredPayload{
				UserID: 1, PredictionID: 2, MatchID: 3,
				HomeTeam: "Mexico", AwayTeam: "Brazil",
				HomeScore: 2, AwayScore: 1, PointsEarned: 5,
			},
			target: func() any { return &notification.PredictionScoredPayload{} },
			check:  checkPredictionScored,
		},
		{
			name: "AdminBankTransferPayload",
			payload: notification.AdminBankTransferPayload{
				ProofID:     77,
				UserID:      12,
				AmountCents: 100000,
				Currency:    "GTQ",
			},
			target: func() any { return &notification.AdminBankTransferPayload{} },
			check:  checkAdminBankTransfer,
		},
		{
			name: "MatchEventPayload with scores",
			payload: func() notification.MatchEventPayload {
				h, a := 2, 1
				return notification.MatchEventPayload{
					MatchID: 10, HomeTeam: "A", AwayTeam: "B",
					HomeScore: &h, AwayScore: &a,
				}
			}(),
			target: func() any { return &notification.MatchEventPayload{} },
			check:  checkMatchEventHomeScore,
		},
		{
			name: "PredictionDeadlinePayload",
			payload: notification.PredictionDeadlinePayload{
				UserID:      5,
				MatchID:     20,
				HomeTeam:    "France",
				AwayTeam:    "Germany",
				DeadlineAt:  time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC),
				MinutesLeft: 60,
			},
			target: func() any { return &notification.PredictionDeadlinePayload{} },
			check:  checkPredictionDeadline,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertJSONRoundtrip(t, tc.payload, tc.target, tc.check)
		})
	}
}

func assertJSONRoundtrip(t *testing.T, payload any, target func() any, check func(*testing.T, any)) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	dst := target()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	check(t, dst)
}

func checkPredictionScored(t *testing.T, decoded any) {
	t.Helper()
	d := decoded.(*notification.PredictionScoredPayload)
	if d.PointsEarned != 5 {
		t.Errorf("PointsEarned: got %d; want 5", d.PointsEarned)
	}
}

func checkAdminBankTransfer(t *testing.T, decoded any) {
	t.Helper()
	d := decoded.(*notification.AdminBankTransferPayload)
	if d.ProofID != 77 {
		t.Errorf("ProofID: got %d; want 77", d.ProofID)
	}
}

func checkMatchEventHomeScore(t *testing.T, decoded any) {
	t.Helper()
	d := decoded.(*notification.MatchEventPayload)
	if d.HomeScore == nil || *d.HomeScore != 2 {
		t.Errorf("HomeScore: got %v; want 2", d.HomeScore)
	}
}

func checkPredictionDeadline(t *testing.T, decoded any) {
	t.Helper()
	d := decoded.(*notification.PredictionDeadlinePayload)
	if d.MinutesLeft != 60 {
		t.Errorf("MinutesLeft: got %d; want 60", d.MinutesLeft)
	}
}
