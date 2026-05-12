package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

func seedWithdrawalRequest(t *testing.T, userID, amountCents int) *domain.WithdrawalRequest {
	t.Helper()
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)
	req := &domain.WithdrawalRequest{
		UserID:        userID,
		AmountCents:   amountCents,
		Currency:      "GTQ",
		Method:        domain.WithdrawalMethodBankGT,
		PayoutDetails: map[string]string{"account": "1234567890"},
	}
	if err := repo.CreateAndReserve(context.Background(), req); err != nil {
		t.Fatalf("seedWithdrawalRequest: %v", err)
	}
	return req
}

func TestWithdrawalRequestRepository_CreateAndReserve_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	req := seedWithdrawalRequest(t, u.ID, 5000)

	if req.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if req.Status != domain.WithdrawalPending {
		t.Errorf("status: got %q, want pending", req.Status)
	}
}

func TestWithdrawalRequestRepository_CreateAndReserve_ReservesBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	seedWithdrawalRequest(t, u.ID, 3000)

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 10000 {
		t.Errorf("balance_cents unchanged: got %d, want 10000", bal)
	}
	if reserved != 3000 {
		t.Errorf("reserved_cents: got %d, want 3000", reserved)
	}
}

func TestWithdrawalRequestRepository_CreateAndReserve_InsufficientBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 1000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	req := &domain.WithdrawalRequest{
		UserID: u.ID, AmountCents: 5000, Currency: "GTQ",
		Method: domain.WithdrawalMethodBankGT,
	}
	err := repo.CreateAndReserve(context.Background(), req)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for insufficient balance, got %v", err)
	}
}

func TestWithdrawalRequestRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	created := seedWithdrawalRequest(t, u.ID, 2000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	got, err := repo.GetByID(context.Background(), int(created.ID))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("expected ID %d, got %v", created.ID, got)
	}
}

func TestWithdrawalRequestRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	got, err := repo.GetByID(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestWithdrawalRequestRepository_ListByUser_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 30000)
	u2 := seedUserWithBalance(t, 5000)
	seedWithdrawalRequest(t, u.ID, 1000)
	seedWithdrawalRequest(t, u.ID, 2000)
	seedWithdrawalRequest(t, u2.ID, 3000)

	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)
	results, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 requests for user %d, got %d", u.ID, len(results))
	}
}

func TestWithdrawalRequestRepository_ListPending_ReturnsOnlyPending(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 20000)
	admin := seedUser(t)
	seedWithdrawalRequest(t, u.ID, 1000)
	seedWithdrawalRequest(t, u.ID, 2000)
	req := seedWithdrawalRequest(t, u.ID, 3000)

	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)
	if _, err := repo.Approve(context.Background(), int(req.ID), admin.ID, "ok"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	pending, err := repo.ListPending(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}
}

func TestWithdrawalRequestRepository_Approve_UpdatesStatus(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	admin := seedUser(t)
	req := seedWithdrawalRequest(t, u.ID, 2000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	approved, err := repo.Approve(context.Background(), int(req.ID), admin.ID, "all good")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if approved.Status != domain.WithdrawalApproved {
		t.Errorf("status: got %q, want approved", approved.Status)
	}
}

func TestWithdrawalRequestRepository_Approve_NotFound(t *testing.T) {
	cleanTables(t)
	admin := seedUser(t)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	_, err := repo.Approve(context.Background(), 999999, admin.ID, "")
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}

func TestWithdrawalRequestRepository_RejectAndRelease_ReleasesReservation(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	admin := seedUser(t)
	req := seedWithdrawalRequest(t, u.ID, 4000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	rejected, err := repo.RejectAndRelease(context.Background(), int(req.ID), admin.ID, "invalid account")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rejected.Status != domain.WithdrawalRejected {
		t.Errorf("status: got %q, want rejected", rejected.Status)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	_, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if reserved != 0 {
		t.Errorf("reserved_cents after reject: got %d, want 0", reserved)
	}
}

func TestWithdrawalRequestRepository_RejectAndRelease_NotFound(t *testing.T) {
	cleanTables(t)
	admin := seedUser(t)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	_, err := repo.RejectAndRelease(context.Background(), 999999, admin.ID, "notes")
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}

func TestWithdrawalRequestRepository_MarkProcessedAndCommit_DeductsBalance(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	admin := seedUser(t)
	req := seedWithdrawalRequest(t, u.ID, 4000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	if _, err := repo.Approve(context.Background(), int(req.ID), admin.ID, "ok"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	processed, err := repo.MarkProcessedAndCommit(context.Background(), int(req.ID))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if processed.Status != domain.WithdrawalProcessed {
		t.Errorf("status: got %q, want processed", processed.Status)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, reserved, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 6000 {
		t.Errorf("balance_cents after process: got %d, want 6000", bal)
	}
	if reserved != 0 {
		t.Errorf("reserved_cents after process: got %d, want 0", reserved)
	}
}

func TestWithdrawalRequestRepository_MarkProcessedAndCommit_NotApproved_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUserWithBalance(t, 10000)
	req := seedWithdrawalRequest(t, u.ID, 2000)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	_, err := repo.MarkProcessedAndCommit(context.Background(), int(req.ID))
	if err == nil {
		t.Fatal("expected conflict error for pending withdrawal, got nil")
	}
	if !errors.Is(err, apperrors.ErrConflict) && !isNotFound(err) {
		t.Errorf("expected conflict or not-found, got %v", err)
	}
}

func TestWithdrawalRequestRepository_MarkProcessedAndCommit_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresWithdrawalRequestRepository(testDB)

	_, err := repo.MarkProcessedAndCommit(context.Background(), 999999)
	if !isNotFound(err) {
		t.Errorf("expected not-found, got %v", err)
	}
}
