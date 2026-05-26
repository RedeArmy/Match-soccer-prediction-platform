package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type bankTransferProofRepoStub struct {
	proof  *domain.BankTransferProof
	proofs []*domain.BankTransferProof
	err    error
}

func (r *bankTransferProofRepoStub) Create(_ context.Context, proof *domain.BankTransferProof) error {
	if r.err != nil {
		return r.err
	}
	proof.ID = 99
	return nil
}
func (r *bankTransferProofRepoStub) GetByID(_ context.Context, _ int) (*domain.BankTransferProof, error) {
	return r.proof, r.err
}
func (r *bankTransferProofRepoStub) ListByUser(_ context.Context, _ int) ([]*domain.BankTransferProof, error) {
	return r.proofs, r.err
}
func (r *bankTransferProofRepoStub) ListPending(_ context.Context) ([]*domain.BankTransferProof, error) {
	return r.proofs, r.err
}
func (r *bankTransferProofRepoStub) ApproveAndCredit(_ context.Context, _ int, _ int, _ string) (*domain.BankTransferProof, error) {
	return r.proof, r.err
}
func (r *bankTransferProofRepoStub) Reject(_ context.Context, _ int, _ int, _ string) (*domain.BankTransferProof, error) {
	return r.proof, r.err
}

func newBankTransferSvc(repo *bankTransferProofRepoStub) BankTransferService {
	return NewBankTransferService(repo, NoopKYCGate{}, nil, &noopAuditLogger{}, zap.NewNop())
}

func newBankTransferSvcWithOutbox(repo *bankTransferProofRepoStub, w *stubOutboxWriter) BankTransferService {
	return NewBankTransferService(repo, NoopKYCGate{}, w, &noopAuditLogger{}, zap.NewNop())
}

// ── Upload ────────────────────────────────────────────────────────────────────

func TestBankTransferService_Upload_HappyPath_ReturnsProof(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{})

	got, err := svc.Upload(context.Background(), 1, 5000, "GTQ", "bank-transfers/1/abc.jpg", "image/jpeg", 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 99 {
		t.Errorf("expected proof with ID 99, got %v", got)
	}
}

func TestBankTransferService_Upload_DefaultsCurrencyToGTQ(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{})

	got, err := svc.Upload(context.Background(), 1, 1000, "", "key", "image/png", 512)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Currency != "GTQ" {
		t.Errorf("currency: got %q, want GTQ", got.Currency)
	}
}

func TestBankTransferService_Upload_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{})

	_, err := svc.Upload(context.Background(), 1, 0, "GTQ", "key", "image/jpeg", 1024)
	if err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestBankTransferService_Upload_NegativeAmountReturnsValidation(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{})

	_, err := svc.Upload(context.Background(), 1, -100, "GTQ", "key", "image/jpeg", 1024)
	if err == nil {
		t.Fatal("expected validation error for negative amount, got nil")
	}
}

func TestBankTransferService_Upload_EmptyStorageKeyReturnsValidation(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{})

	_, err := svc.Upload(context.Background(), 1, 5000, "GTQ", "", "image/jpeg", 1024)
	if err == nil {
		t.Fatal("expected validation error for empty storage key, got nil")
	}
}

func TestBankTransferService_Upload_RepoErrorPropagates(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{err: errors.New("db error")})

	_, err := svc.Upload(context.Background(), 1, 5000, "GTQ", "key", "image/jpeg", 1024)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestBankTransferService_GetByID_ReturnsProof(t *testing.T) {
	proof := &domain.BankTransferProof{ID: 7, AmountCents: 3000}
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proof: proof})

	got, err := svc.GetByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Errorf("expected proof ID 7, got %v", got)
	}
}

func TestBankTransferService_GetByID_NotFound(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proof: nil})

	got, err := svc.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for not found, got %v", got)
	}
}

// ── ListByUser ────────────────────────────────────────────────────────────────

func TestBankTransferService_ListByUser_ReturnsAll(t *testing.T) {
	proofs := []*domain.BankTransferProof{{ID: 1}, {ID: 2}}
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proofs: proofs})

	got, err := svc.ListByUser(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 proofs, got %d", len(got))
	}
}

// ── ListPending ───────────────────────────────────────────────────────────────

func TestBankTransferService_ListPending_ReturnsAll(t *testing.T) {
	proofs := []*domain.BankTransferProof{{ID: 3, Status: domain.BankTransferPending}}
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proofs: proofs})

	got, err := svc.ListPending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 pending proof, got %d", len(got))
	}
}

// ── ApproveTransfer ───────────────────────────────────────────────────────────

func TestBankTransferService_ApproveTransfer_HappyPath(t *testing.T) {
	approved := &domain.BankTransferProof{ID: 1, Status: domain.BankTransferApproved, UserID: 5, AmountCents: 5000}
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proof: approved})

	got, err := svc.ApproveTransfer(context.Background(), 1, 99, "valid receipt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.BankTransferApproved {
		t.Errorf("status: got %q, want approved", got.Status)
	}
}

func TestBankTransferService_ApproveTransfer_RepoErrorPropagates(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{err: errors.New("not found")})

	_, err := svc.ApproveTransfer(context.Background(), 999, 99, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── RejectTransfer ────────────────────────────────────────────────────────────

func TestBankTransferService_RejectTransfer_HappyPath(t *testing.T) {
	rejected := &domain.BankTransferProof{ID: 2, Status: domain.BankTransferRejected}
	svc := newBankTransferSvc(&bankTransferProofRepoStub{proof: rejected})

	got, err := svc.RejectTransfer(context.Background(), 2, 99, "blurry image")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.BankTransferRejected {
		t.Errorf("status: got %q, want rejected", got.Status)
	}
}

func TestBankTransferService_RejectTransfer_RepoErrorPropagates(t *testing.T) {
	svc := newBankTransferSvc(&bankTransferProofRepoStub{err: errors.New("already reviewed")})

	_, err := svc.RejectTransfer(context.Background(), 2, 99, "reason")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── writeOutbox ───────────────────────────────────────────────────────────────

func TestBankTransferService_Upload_OutboxWriteError_StillReturnsProof(t *testing.T) {
	w := &stubOutboxWriter{err: errors.New("outbox unavailable")}
	svc := newBankTransferSvcWithOutbox(&bankTransferProofRepoStub{}, w)

	got, err := svc.Upload(context.Background(), 1, 5000, "GTQ", "key", "image/jpeg", 1024)
	if err != nil {
		t.Fatalf("expected nil error even when outbox write fails, got %v", err)
	}
	if got == nil {
		t.Fatal("expected proof, got nil")
	}
	if w.writes != 1 {
		t.Errorf("expected 1 outbox write attempt, got %d", w.writes)
	}
}
