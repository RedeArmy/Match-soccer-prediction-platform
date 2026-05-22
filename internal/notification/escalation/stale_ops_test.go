package escalation_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/escalation"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubStore struct {
	transfers []*domain.BankTransferProof
	withdraws []*domain.WithdrawalRequest
	listErr   error
}

func (s *stubStore) ListStaleBankTransfers(_ context.Context, _ time.Time) ([]*domain.BankTransferProof, error) {
	return s.transfers, s.listErr
}
func (s *stubStore) ListStaleWithdrawals(_ context.Context, _ time.Time) ([]*domain.WithdrawalRequest, error) {
	return s.withdraws, s.listErr
}

type writtenEvent struct {
	eventType     notification.EventType
	aggregateType string
	aggregateID   string
	payload       any
}

type stubWriter struct {
	events   []writtenEvent
	writeErr error
}

func (w *stubWriter) Write(_ context.Context, et notification.EventType, at, ai string, payload any) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	w.events = append(w.events, writtenEvent{et, at, ai, payload})
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return fixedNow }

func newProof(id int64, createdAt time.Time) *domain.BankTransferProof {
	return &domain.BankTransferProof{
		ID:          id,
		UserID:      1,
		AmountCents: 10000,
		Currency:    "GTQ",
		CreatedAt:   createdAt,
	}
}

func newWithdrawal(id int, createdAt time.Time) *domain.WithdrawalRequest {
	return &domain.WithdrawalRequest{
		ID:          id,
		UserID:      1,
		AmountCents: 5000,
		Currency:    "GTQ",
		CreatedAt:   createdAt,
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestStaleOps_Run_EmitsEventPerStaleBankTransfer(t *testing.T) {
	store := &stubStore{
		transfers: []*domain.BankTransferProof{
			newProof(1, fixedNow.Add(-25*time.Hour)),
			newProof(2, fixedNow.Add(-30*time.Hour)),
		},
	}
	writer := &stubWriter{}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: 24 * time.Hour,
		Now:               fixedClock,
	}, zap.NewNop())

	if err := e.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writer.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(writer.events))
	}
	for _, ev := range writer.events {
		if ev.eventType != notification.EventAdminBankTransferStale {
			t.Errorf("expected EventAdminBankTransferStale, got %q", ev.eventType)
		}
	}
	// aggregate IDs must match proof IDs
	if writer.events[0].aggregateID != "1" || writer.events[1].aggregateID != "2" {
		t.Errorf("aggregate IDs mismatch: %v", writer.events)
	}
}

func TestStaleOps_Run_EmitsEventPerStaleWithdrawal(t *testing.T) {
	store := &stubStore{
		withdraws: []*domain.WithdrawalRequest{
			newWithdrawal(10, fixedNow.Add(-50*time.Hour)),
		},
	}
	writer := &stubWriter{}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		WithdrawalStale: 48 * time.Hour,
		Now:             fixedClock,
	}, zap.NewNop())

	if err := e.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(writer.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(writer.events))
	}
	if writer.events[0].eventType != notification.EventAdminWithdrawalStale {
		t.Errorf("expected EventAdminWithdrawalStale, got %q", writer.events[0].eventType)
	}
	if writer.events[0].aggregateID != "10" {
		t.Errorf("expected aggregateID '10', got %q", writer.events[0].aggregateID)
	}
}

func TestStaleOps_Run_SkipsChecksWhenThresholdIsZero(t *testing.T) {
	store := &stubStore{listErr: errors.New("should not be called")}
	writer := &stubWriter{}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: 0, // disabled
		WithdrawalStale:   0, // disabled
		Now:               fixedClock,
	}, zap.NewNop())

	if err := e.Run(context.Background()); err != nil {
		t.Fatalf("expected no error when thresholds are zero, got: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("expected no events, got %d", len(writer.events))
	}
}

func TestStaleOps_Run_ReturnsErrorOnListFailure(t *testing.T) {
	store := &stubStore{listErr: errors.New("db down")}
	writer := &stubWriter{}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: 24 * time.Hour,
		Now:               fixedClock,
	}, zap.NewNop())

	if err := e.Run(context.Background()); err == nil {
		t.Fatal("expected error from list failure, got nil")
	}
}

func TestStaleOps_Run_ContinuesAfterItemWriteFailure(t *testing.T) {
	store := &stubStore{
		transfers: []*domain.BankTransferProof{
			newProof(1, fixedNow.Add(-25*time.Hour)),
			newProof(2, fixedNow.Add(-26*time.Hour)),
		},
	}
	// Writer always fails but Run should not propagate per-item failures.
	writer := &stubWriter{writeErr: errors.New("outbox full")}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: 24 * time.Hour,
		Now:               fixedClock,
	}, zap.NewNop())

	if err := e.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error on per-item write failure, got: %v", err)
	}
}

func TestStaleOps_Run_UsesCorrectCutoffTime(t *testing.T) {
	var capturedBefore time.Time
	store := &captureStore{captureBankFn: func(before time.Time) { capturedBefore = before }}
	writer := &stubWriter{}
	stale := 6 * time.Hour
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: stale,
		Now:               fixedClock,
	}, zap.NewNop())

	_ = e.Run(context.Background())

	expected := fixedNow.Add(-stale)
	if !capturedBefore.Equal(expected) {
		t.Errorf("cutoff: want %v, got %v", expected, capturedBefore)
	}
}

// captureStore records the before-cutoff values passed to list methods.
type captureStore struct {
	captureBankFn func(time.Time)
}

func (s *captureStore) ListStaleBankTransfers(_ context.Context, before time.Time) ([]*domain.BankTransferProof, error) {
	if s.captureBankFn != nil {
		s.captureBankFn(before)
	}
	return nil, nil
}
func (s *captureStore) ListStaleWithdrawals(_ context.Context, _ time.Time) ([]*domain.WithdrawalRequest, error) {
	return nil, nil
}

func TestStaleOps_Run_PayloadContainsPendingSince(t *testing.T) {
	pendingAt := fixedNow.Add(-36 * time.Hour)
	store := &stubStore{
		transfers: []*domain.BankTransferProof{newProof(5, pendingAt)},
	}
	writer := &stubWriter{}
	e := escalation.NewStaleOps(store, writer, escalation.Config{
		BankTransferStale: 24 * time.Hour,
		Now:               fixedClock,
	}, zap.NewNop())

	_ = e.Run(context.Background())

	if len(writer.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(writer.events))
	}
	p, ok := writer.events[0].payload.(notification.AdminBankTransferPayload)
	if !ok {
		t.Fatalf("payload type: want AdminBankTransferPayload, got %T", writer.events[0].payload)
	}
	want := pendingAt.UTC().Format(time.RFC3339)
	if p.PendingSince != want {
		t.Errorf("PendingSince: want %q, got %q", want, p.PendingSince)
	}
	if p.ProofID != 5 {
		t.Errorf("ProofID: want 5, got %d", p.ProofID)
	}
	if got := fmt.Sprintf("%d", p.ProofID); got != writer.events[0].aggregateID {
		t.Errorf("aggregateID: want %q, got %q", got, writer.events[0].aggregateID)
	}
}
