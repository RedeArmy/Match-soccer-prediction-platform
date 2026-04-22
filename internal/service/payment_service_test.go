package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// stubPaymentRepo implements repository.PaymentRecordRepository for unit tests.
type stubPaymentRepo struct {
	record  *domain.PaymentRecord
	records []*domain.PaymentRecord
	err     error
}

func (r *stubPaymentRepo) Create(_ context.Context, record *domain.PaymentRecord) error {
	if r.err != nil {
		return r.err
	}
	record.ID = 99
	return nil
}
func (r *stubPaymentRepo) GetByID(_ context.Context, _ int) (*domain.PaymentRecord, error) {
	return r.record, r.err
}
func (r *stubPaymentRepo) ListByQuiniela(_ context.Context, _ int, _ repository.PaymentFilters) ([]*domain.PaymentRecord, error) {
	return r.records, r.err
}
func (r *stubPaymentRepo) ListByUser(_ context.Context, _ int) ([]*domain.PaymentRecord, error) {
	return r.records, r.err
}
func (r *stubPaymentRepo) ListPending(_ context.Context) ([]*domain.PaymentRecord, error) {
	return r.records, r.err
}
func (r *stubPaymentRepo) Validate(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return r.record, r.err
}
func (r *stubPaymentRepo) Reject(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return r.record, r.err
}

func newPaymentSvc(repo *stubPaymentRepo) PaymentService {
	return NewPaymentService(repo, &noopAuditLogger{}, zap.NewNop())
}

// ── CreateRecord ──────────────────────────────────────────────────────────────

func TestPaymentService_CreateRecord_HappyPath_ReturnsRecord(t *testing.T) {
	svc := newPaymentSvc(&stubPaymentRepo{})

	got, err := svc.CreateRecord(context.Background(), 1, 2, 500, "USD", "ref-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 99 {
		t.Errorf("expected record with ID 99, got %v", got)
	}
}

func TestPaymentService_CreateRecord_RepoError_Propagates(t *testing.T) {
	svc := newPaymentSvc(&stubPaymentRepo{err: errors.New("db error")})

	_, err := svc.CreateRecord(context.Background(), 1, 2, 500, "USD", "ref")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ValidateDeposit ───────────────────────────────────────────────────────────

func TestPaymentService_ValidateDeposit_HappyPath_ReturnsRecord(t *testing.T) {
	rec := &domain.PaymentRecord{ID: 5}
	svc := newPaymentSvc(&stubPaymentRepo{record: rec})

	got, err := svc.ValidateDeposit(context.Background(), 5, 99, "approved")
	if err != nil || got == nil {
		t.Fatalf("expected record, got %v err=%v", got, err)
	}
}

func TestPaymentService_ValidateDeposit_RepoError_Propagates(t *testing.T) {
	svc := newPaymentSvc(&stubPaymentRepo{err: errors.New("not pending")})

	_, err := svc.ValidateDeposit(context.Background(), 5, 99, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── RejectDeposit ─────────────────────────────────────────────────────────────

func TestPaymentService_RejectDeposit_HappyPath_ReturnsRecord(t *testing.T) {
	rec := &domain.PaymentRecord{ID: 7}
	svc := newPaymentSvc(&stubPaymentRepo{record: rec})

	got, err := svc.RejectDeposit(context.Background(), 7, 99, "invalid")
	if err != nil || got == nil {
		t.Fatalf("expected record, got %v err=%v", got, err)
	}
}

func TestPaymentService_RejectDeposit_RepoError_Propagates(t *testing.T) {
	svc := newPaymentSvc(&stubPaymentRepo{err: errors.New("not found")})

	_, err := svc.RejectDeposit(context.Background(), 7, 99, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ListPending / ListByQuiniela ──────────────────────────────────────────────

func TestPaymentService_ListPending_ReturnsPendingRecords(t *testing.T) {
	recs := []*domain.PaymentRecord{{ID: 1}, {ID: 2}}
	svc := newPaymentSvc(&stubPaymentRepo{records: recs})

	got, err := svc.ListPending(context.Background())
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 records, got %v err=%v", got, err)
	}
}

func TestPaymentService_ListByQuiniela_ReturnsRecords(t *testing.T) {
	recs := []*domain.PaymentRecord{{ID: 3}}
	svc := newPaymentSvc(&stubPaymentRepo{records: recs})

	got, err := svc.ListByQuiniela(context.Background(), 1)
	if err != nil || len(got) != 1 {
		t.Errorf("expected 1 record, got %v err=%v", got, err)
	}
}
