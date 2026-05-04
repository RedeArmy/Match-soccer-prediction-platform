package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── PaymentRecordRepository ───────────────────────────────────────────────────

func TestPaymentRecordRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)

	pr := seedPaymentRecord(t, q.ID, u.ID)
	if pr.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if pr.Status != domain.PaymentStatusPending {
		t.Errorf("expected pending status, got %q", pr.Status)
	}
}

func TestPaymentRecordRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	created := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("expected ID %d, got %v", created.ID, got)
	}
}

func TestPaymentRecordRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	got, err := repo.GetByID(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestPaymentRecordRepository_ListByQuiniela(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	u2 := seedUser(t)
	q2 := seedQuiniela(t, u2.ID)
	seedPaymentRecord(t, q2.ID, u2.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	results, err := repo.ListByQuiniela(context.Background(), q.ID, repository.PaymentFilters{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records for quiniela %d, got %d", q.ID, len(results))
	}
}

func TestPaymentRecordRepository_ListByQuiniela_FilterByStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	admin := seedUser(t)
	pr1 := seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr1.ID, admin.ID, "ok")

	status := domain.PaymentStatusPending
	pending, err := repo.ListByQuiniela(context.Background(), q.ID, repository.PaymentFilters{Status: &status})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending record, got %d", len(pending))
	}
}

func TestPaymentRecordRepository_ListByUser(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	results, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records for user %d, got %d", u.ID, len(results))
	}
}

func TestPaymentRecordRepository_ListPending(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	admin := seedUser(t)
	pr1 := seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr1.ID, admin.ID, "paid")

	pending, err := repo.ListPending(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending record, got %d", len(pending))
	}
}

func TestPaymentRecordRepository_Validate_TransitionsToConfirmed(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	result, err := repo.Validate(context.Background(), pr.ID, admin.ID, "verified manually")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result.Status != domain.PaymentStatusConfirmed {
		t.Errorf("expected confirmed, got %q", result.Status)
	}
	if result.ReviewedBy == nil || *result.ReviewedBy != admin.ID {
		t.Errorf("expected reviewed_by %d, got %v", admin.ID, result.ReviewedBy)
	}
	if result.ConfirmedAt == nil {
		t.Error("expected confirmed_at to be set")
	}
}

func TestPaymentRecordRepository_Reject_TransitionsToRejected(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	result, err := repo.Reject(context.Background(), pr.ID, admin.ID, repoFakeReceipt)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if result.Status != domain.PaymentStatusRejected {
		t.Errorf("expected rejected, got %q", result.Status)
	}
	if result.Notes != repoFakeReceipt {
		t.Errorf("expected notes %q, got %q", repoFakeReceipt, result.Notes)
	}
}

func TestPaymentRecordRepository_Validate_NotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	_, err := repo.Validate(context.Background(), 999999, u.ID, "")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestPaymentRecordRepository_Reject_AlreadyConfirmedIsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	_, _ = repo.Validate(context.Background(), pr.ID, admin.ID, "ok")

	_, err := repo.Reject(context.Background(), pr.ID, admin.ID, "late reject")
	if !isNotFound(err) {
		t.Errorf("expected not-found for confirmed payment reject, got %v", err)
	}
}

// ── PaymentRecordRepository admin extensions ──────────────────────────────────

func TestPaymentRecordRepository_List_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{}, repository.Unbounded())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records, got %d", len(results))
	}
}

func TestPaymentRecordRepository_List_FilterByQuinielaID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q1.ID, u.ID)
	seedPaymentRecord(t, q2.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{QuinielaID: &q1.ID}, repository.Unbounded())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 record for quiniela %d, got %d", q1.ID, len(results))
	}
}

func TestPaymentRecordRepository_List_Pagination(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	results, err := repo.List(context.Background(), repository.PaymentFilters{}, repository.Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 records with limit=2 offset=1, got %d", len(results))
	}
}

func TestPaymentRecordRepository_ListStale_ReturnsPending(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)

	stale, err := repo.ListStale(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 1 {
		t.Errorf("expected 1 stale payment, got %d", len(stale))
	}
}

func TestPaymentRecordRepository_ListStale_ExcludesConfirmed(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	pr := seedPaymentRecord(t, q.ID, u.ID)
	repo := repository.NewPostgresPaymentRecordRepository(testDB)
	_, _ = repo.Validate(context.Background(), pr.ID, admin.ID, "ok")

	stale, err := repo.ListStale(context.Background(), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale (confirmed excluded), got %d", len(stale))
	}
}

// ── PaymentRecordRepository.GetStatusCounts ───────────────────────────────────

func TestPaymentRecordRepository_GetStatusCounts_IncludesTotalCollected(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	member := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	paymentRepo := repository.NewPostgresPaymentRecordRepository(testDB)

	pr1 := seedPaymentRecord(t, q.ID, member.ID)
	pr2 := seedPaymentRecord(t, q.ID, owner.ID)

	if _, err := paymentRepo.Validate(context.Background(), pr2.ID, admin.ID, "ok"); err != nil {
		t.Fatalf("validate: %v", err)
	}

	counts, err := paymentRepo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Pending < 1 {
		t.Errorf("expected Pending ≥ 1, got %d", counts.Pending)
	}
	if counts.Confirmed < 1 {
		t.Errorf("expected Confirmed ≥ 1, got %d", counts.Confirmed)
	}
	if counts.TotalCollected < 10000 {
		t.Errorf("expected TotalCollected ≥ 10000 (one confirmed at 10000 centavos), got %d", counts.TotalCollected)
	}
	_ = pr1
}
