package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

func seedBankTransferProof(t *testing.T, userID, amountCents int) *domain.BankTransferProof {
	t.Helper()
	repo := repository.NewPostgresBankTransferProofRepository(testDB)
	p := &domain.BankTransferProof{
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    "GTQ",
		StorageKey:  "bank-transfers/" + nextCode() + "/proof.jpg",
		ContentType: "image/jpeg",
		FileSize:    1024,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seedBankTransferProof: %v", err)
	}
	return p
}

func TestBankTransferProofRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	proof := seedBankTransferProof(t, u.ID, 5000)

	if proof.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if proof.Status != domain.BankTransferPending {
		t.Errorf("status: got %q, want pending", proof.Status)
	}
}

func TestBankTransferProofRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	created := seedBankTransferProof(t, u.ID, 3000)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	got, err := repo.GetByID(context.Background(), int(created.ID))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("expected ID %d, got %v", created.ID, got)
	}
	if got.AmountCents != 3000 {
		t.Errorf("amount_cents: got %d, want 3000", got.AmountCents)
	}
}

func TestBankTransferProofRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	got, err := repo.GetByID(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestBankTransferProofRepository_ListByUser_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	u2 := seedUser(t)
	seedBankTransferProof(t, u.ID, 1000)
	seedBankTransferProof(t, u.ID, 2000)
	seedBankTransferProof(t, u2.ID, 3000)

	repo := repository.NewPostgresBankTransferProofRepository(testDB)
	results, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 proofs for user %d, got %d", u.ID, len(results))
	}
}

func TestBankTransferProofRepository_ListPending_ReturnsOnlyPending(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	seedBankTransferProof(t, u.ID, 1000) // pending
	seedBankTransferProof(t, u.ID, 2000) // pending
	p := seedBankTransferProof(t, u.ID, 3000)

	repo := repository.NewPostgresBankTransferProofRepository(testDB)
	// Approve one proof so it's no longer pending.
	if _, err := repo.ApproveAndCredit(context.Background(), int(p.ID), admin.ID, "ok"); err != nil {
		t.Fatalf("ApproveAndCredit: %v", err)
	}

	pending, err := repo.ListPending(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending proofs, got %d", len(pending))
	}
}

func TestBankTransferProofRepository_ApproveAndCredit_UpdatesStatusAndBalance(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	proof := seedBankTransferProof(t, u.ID, 5000)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	approved, err := repo.ApproveAndCredit(context.Background(), int(proof.ID), admin.ID, "valid receipt")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if approved.Status != domain.BankTransferApproved {
		t.Errorf("status: got %q, want approved", approved.Status)
	}

	// Verify balance was credited.
	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 5000 {
		t.Errorf("balance_cents after approve: got %d, want 5000", bal)
	}
}

func TestBankTransferProofRepository_ApproveAndCredit_NotFound(t *testing.T) {
	cleanTables(t)
	admin := seedUser(t)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	_, err := repo.ApproveAndCredit(context.Background(), 999999, admin.ID, "")
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}

func TestBankTransferProofRepository_ApproveAndCredit_AlreadyApproved_Idempotent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	proof := seedBankTransferProof(t, u.ID, 1000)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	if _, err := repo.ApproveAndCredit(context.Background(), int(proof.ID), admin.ID, "ok"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	// Second approve should return existing (idempotent) or conflict — not an error panic.
	result, err := repo.ApproveAndCredit(context.Background(), int(proof.ID), admin.ID, "ok")
	if err != nil && !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected idempotent result or conflict, got %v", err)
	}
	if result != nil && result.Status != domain.BankTransferApproved {
		t.Errorf("status: got %q, want approved", result.Status)
	}
}

func TestBankTransferProofRepository_Reject_UpdatesStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	proof := seedBankTransferProof(t, u.ID, 2000)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	rejected, err := repo.Reject(context.Background(), int(proof.ID), admin.ID, "blurry image")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rejected.Status != domain.BankTransferRejected {
		t.Errorf("status: got %q, want rejected", rejected.Status)
	}
}

func TestBankTransferProofRepository_Reject_NotFound(t *testing.T) {
	cleanTables(t)
	admin := seedUser(t)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	_, err := repo.Reject(context.Background(), 999999, admin.ID, "not found")
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}

func TestBankTransferProofRepository_Reject_AlreadyRejected_Idempotent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	proof := seedBankTransferProof(t, u.ID, 1000)
	repo := repository.NewPostgresBankTransferProofRepository(testDB)

	if _, err := repo.Reject(context.Background(), int(proof.ID), admin.ID, "bad"); err != nil {
		t.Fatalf("first reject: %v", err)
	}
	result, err := repo.Reject(context.Background(), int(proof.ID), admin.ID, "bad again")
	if err != nil && !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected idempotent result or conflict, got %v", err)
	}
	if result != nil && result.Status != domain.BankTransferRejected {
		t.Errorf("status: got %q, want rejected", result.Status)
	}
}
