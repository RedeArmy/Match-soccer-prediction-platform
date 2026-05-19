package escalation_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/escalation"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

type stubParams struct{ staleSecBank, staleSecWithdraw int }

func (s *stubParams) GetInt(_ context.Context, key string, defaultVal int) int {
	switch key {
	case domain.ParamKeyNotifyBankTransferStaleSec:
		return s.staleSecBank
	case domain.ParamKeyNotifyWithdrawalStaleSec:
		return s.staleSecWithdraw
	}
	return defaultVal
}

type stubTransferRepo struct{ proofs []*domain.BankTransferProof }

func (r *stubTransferRepo) ListPending(_ context.Context) ([]*domain.BankTransferProof, error) {
	return r.proofs, nil
}
func (r *stubTransferRepo) Create(_ context.Context, _ *domain.BankTransferProof) error {
	panic("stub: unexpected call")
}
func (r *stubTransferRepo) GetByID(_ context.Context, _ int) (*domain.BankTransferProof, error) {
	panic("stub: unexpected call")
}
func (r *stubTransferRepo) ListByUser(_ context.Context, _ int) ([]*domain.BankTransferProof, error) {
	panic("stub: unexpected call")
}
func (r *stubTransferRepo) ApproveAndCredit(_ context.Context, _, _ int, _ string) (*domain.BankTransferProof, error) {
	panic("stub: unexpected call")
}
func (r *stubTransferRepo) Reject(_ context.Context, _, _ int, _ string) (*domain.BankTransferProof, error) {
	panic("stub: unexpected call")
}

type stubWithdrawalRepo struct{ reqs []*domain.WithdrawalRequest }

func (r *stubWithdrawalRepo) ListPending(_ context.Context) ([]*domain.WithdrawalRequest, error) {
	return r.reqs, nil
}
func (r *stubWithdrawalRepo) CreateAndReserve(_ context.Context, _ *domain.WithdrawalRequest) error {
	panic("stub: unexpected call")
}
func (r *stubWithdrawalRepo) GetByID(_ context.Context, _ int) (*domain.WithdrawalRequest, error) {
	panic("stub: unexpected call")
}
func (r *stubWithdrawalRepo) ListByUser(_ context.Context, _ int) ([]*domain.WithdrawalRequest, error) {
	panic("stub: unexpected call")
}
func (r *stubWithdrawalRepo) Approve(_ context.Context, _, _ int, _ string) (*domain.WithdrawalRequest, error) {
	panic("stub: unexpected call")
}
func (r *stubWithdrawalRepo) RejectAndRelease(_ context.Context, _, _ int, _ string) (*domain.WithdrawalRequest, error) {
	panic("stub: unexpected call")
}
func (r *stubWithdrawalRepo) MarkProcessedAndCommit(_ context.Context, _ int) (*domain.WithdrawalRequest, error) {
	panic("stub: unexpected call")
}

type recordingWriter struct{ events []notification.EventType }

func (w *recordingWriter) Write(_ context.Context, et notification.EventType, _, _ string, _ any) error {
	w.events = append(w.events, et)
	return nil
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestScheduler_StaleTransfer_EmitsEvent(t *testing.T) {
	t.Parallel()

	staleProof := &domain.BankTransferProof{
		ID: 7, UserID: 42, AmountCents: 100000, Currency: "GTQ",
		Status:    domain.BankTransferPending,
		CreatedAt: time.Now().Add(-13 * time.Hour), // 13 h > 12 h threshold
	}
	freshProof := &domain.BankTransferProof{
		ID:        8,
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}

	writer := &recordingWriter{}
	sched := escalation.NewScheduler(
		&stubParams{staleSecBank: 12 * 3600, staleSecWithdraw: 24 * 3600},
		&stubTransferRepo{proofs: []*domain.BankTransferProof{staleProof, freshProof}},
		&stubWithdrawalRepo{},
		writer,
		time.Hour,
		zaptest.NewLogger(t),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // let Run exit after the initial cycle

	sched.Run(ctx)

	if len(writer.events) != 1 {
		t.Fatalf("outbox events: got %d; want 1", len(writer.events))
	}
	if writer.events[0] != notification.EventAdminBankTransferStale {
		t.Errorf("event: got %q; want %q", writer.events[0], notification.EventAdminBankTransferStale)
	}
}

func TestScheduler_StaleWithdrawal_EmitsEvent(t *testing.T) {
	t.Parallel()

	staleReq := &domain.WithdrawalRequest{
		ID: 3, UserID: 9, AmountCents: 50000, Currency: "GTQ",
		Status:    domain.WithdrawalPending,
		CreatedAt: time.Now().Add(-25 * time.Hour), // 25 h > 24 h threshold
	}

	writer := &recordingWriter{}
	sched := escalation.NewScheduler(
		&stubParams{staleSecBank: 12 * 3600, staleSecWithdraw: 24 * 3600},
		&stubTransferRepo{},
		&stubWithdrawalRepo{reqs: []*domain.WithdrawalRequest{staleReq}},
		writer,
		time.Hour,
		zaptest.NewLogger(t),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sched.Run(ctx)

	if len(writer.events) != 1 {
		t.Fatalf("outbox events: got %d; want 1", len(writer.events))
	}
	if writer.events[0] != notification.EventAdminWithdrawalStale {
		t.Errorf("event: got %q; want %q", writer.events[0], notification.EventAdminWithdrawalStale)
	}
}

func TestScheduler_NothingStale_NoEvents(t *testing.T) {
	t.Parallel()

	writer := &recordingWriter{}
	sched := escalation.NewScheduler(
		&stubParams{staleSecBank: 12 * 3600, staleSecWithdraw: 24 * 3600},
		&stubTransferRepo{proofs: []*domain.BankTransferProof{
			{ID: 1, CreatedAt: time.Now().Add(-30 * time.Minute)},
		}},
		&stubWithdrawalRepo{reqs: []*domain.WithdrawalRequest{
			{ID: 2, CreatedAt: time.Now().Add(-1 * time.Hour)},
		}},
		writer,
		time.Hour,
		zaptest.NewLogger(t),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sched.Run(ctx)

	if len(writer.events) != 0 {
		t.Errorf("outbox events: got %d; want 0", len(writer.events))
	}
}
