package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── QuinielaRepository ────────────────────────────────────────────────────────

func TestQuinielaRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Test Pool", OwnerID: u.ID}
	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestQuinielaRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected quiniela, got nil")
	}
	if got.Name != q.Name {
		t.Errorf("name: got %q, want %q", got.Name, q.Name)
	}
}

func TestQuinielaRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing quiniela, got %+v", got)
	}
}

func TestQuinielaRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q.Name = "Renamed Pool"
	if err := repo.Update(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.Name != "Renamed Pool" {
		t.Errorf("name not updated: got %q", q.Name)
	}
}

func TestQuinielaRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	ghost := &domain.Quiniela{ID: 99999, Name: "Ghost", OwnerID: 1}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.Delete(context.Background(), q.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), q.ID)
	if got != nil {
		t.Error("expected quiniela to be deleted")
	}
}

func TestQuinielaRepository_Delete_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.Delete(context.Background(), 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_ListByOwner_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	quinielas, err := repo.ListByOwner(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(quinielas) != 1 {
		t.Errorf("expected 1 quiniela, got %d", len(quinielas))
	}
}

// ── QuinielaRepository - new fields ──────────────────────────────────────────

func TestQuinielaRepository_Create_HydratesInviteCode(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	code := nextCode()
	q := &domain.Quiniela{Name: "Pool A", OwnerID: u.ID, InviteCode: code, Currency: defaultCurrency}

	if err := repo.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if q.InviteCode != code {
		t.Errorf("invite_code: got %q, want %q", q.InviteCode, code)
	}
}

func TestQuinielaRepository_Create_DuplicateName_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := &domain.Quiniela{Name: "Same Name", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	if err := repo.Create(context.Background(), q1); err != nil {
		t.Fatalf("first create: %v", err)
	}
	q2 := &domain.Quiniela{Name: "Same Name", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	err := repo.Create(context.Background(), q2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error for duplicate name, got %v", err)
	}
}

func TestQuinielaRepository_GetByInviteCode_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByInviteCode(context.Background(), q.InviteCode)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected quiniela, got nil")
	}
	if got.ID != q.ID {
		t.Errorf(fmtIDMismatch, got.ID, q.ID)
	}
}

func TestQuinielaRepository_GetByInviteCode_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	got, err := repo.GetByInviteCode(context.Background(), "NOTEXISTS")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown code, got %+v", got)
	}
}

// ── QuinielaRepository - RotateInviteCode ─────────────────────────────────────

func TestQuinielaRepository_RotateInviteCode_UpdatesCode(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	newCode := "NEWCODE001"
	exp := time.Now().Add(30 * 24 * time.Hour).UTC()
	got, err := repo.RotateInviteCode(context.Background(), q.ID, newCode, &exp)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected non-nil quiniela after RotateInviteCode")
	}
	if got.InviteCode != newCode {
		t.Errorf("invite code: got %q, want %q", got.InviteCode, newCode)
	}
	if got.InviteCodeExpiresAt == nil {
		t.Fatal("expected InviteCodeExpiresAt to be set")
	}
}

func TestQuinielaRepository_RotateInviteCode_NotFound_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	exp := time.Now().Add(time.Hour)
	_, err := repo.RotateInviteCode(context.Background(), 99999, "NEWCODE002", &exp)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── QuinielaRepository - UpdateStatus ────────────────────────────────────────

func TestQuinielaRepository_UpdateStatus_SetsStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), q.ID, domain.QuinielaStatusActive); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.Status != domain.QuinielaStatusActive {
		t.Errorf(repoMsgStatusActive, got.Status)
	}
}

func TestQuinielaRepository_UpdateStatus_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), 99999, domain.QuinielaStatusActive); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── QuinielaRepository - CreateWithMembership ─────────────────────────────────

func TestQuinielaRepository_CreateWithMembership_HydratesBothIDs(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Atomic Pool", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	now := time.Now().UTC()
	m := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}

	if err := repo.CreateWithMembership(context.Background(), q, m); err != nil {
		t.Fatalf("CreateWithMembership: %v", err)
	}
	if q.ID == 0 {
		t.Error("expected quiniela ID to be hydrated")
	}
	if m.ID == 0 {
		t.Error("expected membership ID to be hydrated")
	}
	if m.QuinielaID != q.ID {
		t.Errorf("membership.QuinielaID: got %d, want %d", m.QuinielaID, q.ID)
	}
}

func TestQuinielaRepository_CreateWithMembership_QuinielaVisibleAfterCommit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := &domain.Quiniela{Name: "Visible Pool", OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	now := time.Now().UTC()
	m := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}

	if err := repo.CreateWithMembership(context.Background(), q, m); err != nil {
		t.Fatalf("CreateWithMembership: %v", err)
	}

	got, err := repo.GetByID(context.Background(), q.ID)
	if err != nil {
		t.Fatalf("GetByID after CreateWithMembership: %v", err)
	}
	if got == nil || got.Name != q.Name {
		t.Errorf("expected quiniela %q to be visible after commit, got %v", q.Name, got)
	}
}

func TestQuinielaRepository_CreateWithMembership_DuplicateName_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)
	code := nextCode()

	q1 := &domain.Quiniela{Name: "Same Name " + code, OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	now := time.Now().UTC()
	m1 := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now}
	if err := repo.CreateWithMembership(context.Background(), q1, m1); err != nil {
		t.Fatalf("first CreateWithMembership: %v", err)
	}

	q2 := &domain.Quiniela{Name: q1.Name, OwnerID: u.ID, InviteCode: nextCode(), Currency: defaultCurrency}
	now2 := time.Now().UTC()
	m2 := &domain.GroupMembership{UserID: u.ID, Status: domain.MembershipActive, Paid: false, JoinedAt: &now2}
	err := repo.CreateWithMembership(context.Background(), q2, m2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for duplicate name, got %v", err)
	}
}

// ── QuinielaRepository extensions ────────────────────────────────────────────

func TestQuinielaRepository_UpdateGroupSettings_UpdatesEntryFee(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	updated, err := repo.UpdateGroupSettings(context.Background(), q.ID, 5000)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.EntryFee != 5000 {
		t.Errorf("expected EntryFee 5000, got %d", updated.EntryFee)
	}
}

func TestQuinielaRepository_UpdateGroupSettings_ZeroEntryFee(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	updated, err := repo.UpdateGroupSettings(context.Background(), q.ID, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.EntryFee != 0 {
		t.Errorf("expected EntryFee 0, got %d", updated.EntryFee)
	}
}

func TestQuinielaRepository_UpdateGroupSettings_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	_, err := repo.UpdateGroupSettings(context.Background(), 999999, 0)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestQuinielaRepository_DeleteByAdmin_SoftDeletes(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.DeleteByAdmin(context.Background(), q.ID, admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), q.ID)
	if got != nil {
		t.Errorf("expected nil after DeleteByAdmin, got %+v", got)
	}
}

func TestQuinielaRepository_DeleteByAdmin_NotFoundWhenAlreadyDeleted(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	_ = repo.DeleteByAdmin(context.Background(), q.ID, admin.ID)
	err := repo.DeleteByAdmin(context.Background(), q.ID, admin.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found on second delete, got %v", err)
	}
}

func TestQuinielaRepository_Update_ConflictOnDuplicateName(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q2.Name = q1.Name
	err := repo.Update(context.Background(), q2)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

// ── QuinielaRepository admin extensions ──────────────────────────────────────

func TestQuinielaRepository_ListByIDs_ReturnsMatching(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	_ = seedQuiniela(t, u.ID) // not requested
	repo := repository.NewPostgresQuinielaRepository(testDB)

	results, err := repo.ListByIDs(context.Background(), []int{q1.ID, q2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 quinielas, got %d", len(results))
	}
}

func TestQuinielaRepository_ListByIDs_EmptyInput_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	results, err := repo.ListByIDs(context.Background(), []int{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if results != nil {
		t.Errorf("expected nil for empty ids, got %v", results)
	}
}

// ── QuinielaRepository.GetStatusCounts ───────────────────────────────────────

func TestQuinielaRepository_GetStatusCounts_ReturnsCorrectTotals(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	quinielaRepo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	if err := quinielaRepo.DeleteByAdmin(context.Background(), q1.ID, admin.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	counts, err := quinielaRepo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Total < 2 {
		t.Errorf("expected Total ≥ 2, got %d", counts.Total)
	}
	if counts.Deleted < 1 {
		t.Errorf("expected Deleted ≥ 1, got %d", counts.Deleted)
	}
	_ = q2
}

// ── QuinielaRepository.BulkDeleteByAdmin ─────────────────────────────────────

func TestQuinielaRepository_BulkDeleteByAdmin_DeletesAllIDs(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)

	succeeded, err := repo.BulkDeleteByAdmin(context.Background(), []int{q1.ID, q2.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(succeeded))
	}
}

func TestQuinielaRepository_BulkDeleteByAdmin_AlreadyDeletedSkipped(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	q := seedQuiniela(t, u.ID)
	_, _ = repo.BulkDeleteByAdmin(context.Background(), []int{q.ID}, admin.ID)

	succeeded, err := repo.BulkDeleteByAdmin(context.Background(), []int{q.ID}, admin.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(succeeded) != 0 {
		t.Errorf("expected 0 succeeded for already-deleted ID, got %d", len(succeeded))
	}
}

// ── DistributePrizesAtomically ────────────────────────────────────────────────

func TestQuinielaRepository_DistributePrizesAtomically_CreditsWinnersAndMarksDistributed(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	winner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	const prizeAmount = 50_000
	credits := []repository.PrizeCredit{
		{UserID: winner.ID, AmountCents: prizeAmount, RefID: int64(q.ID), RefType: "quiniela"},
	}

	if err := repo.DistributePrizesAtomically(context.Background(), q.ID, credits, nil); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// prizes_distributed_at must be set.
	var distributedAt *string
	err := testDB.QueryRow(context.Background(),
		`SELECT prizes_distributed_at::text FROM quinielas WHERE id = $1`, q.ID,
	).Scan(&distributedAt)
	if err != nil {
		t.Fatalf("query prizes_distributed_at: %v", err)
	}
	if distributedAt == nil {
		t.Error("expected prizes_distributed_at to be set")
	}

	// winners balance must increase.
	userRepo := repository.NewPostgresUserRepository(testDB)
	balance, _, err := userRepo.GetBalance(context.Background(), winner.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != prizeAmount {
		t.Errorf("balance_cents: got %d, want %d", balance, prizeAmount)
	}

	// A ledger row with kind='prize' must exist.
	var count int
	err = testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM balance_ledger WHERE user_id = $1 AND kind = 'prize'`, winner.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("balance_ledger query: %v", err)
	}
	if count != 1 {
		t.Errorf("balance_ledger rows: got %d, want 1", count)
	}
}

func TestQuinielaRepository_DistributePrizesAtomically_FreezesKYCGatedWinners(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	gatedUser := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	kycRepo := repository.NewPostgresKYCProfileRepository(testDB)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	// gatedUser must have a KYC profile so applyPrizeFreezeTx can UPDATE it.
	now := time.Now()
	docType := domain.KYCDocGovID
	p := &domain.KYCProfile{
		UserID: gatedUser.ID, Status: domain.KYCStatusPending, Tier: domain.KYCTierUnverified,
		FullName: "Gated User", DocumentType: &docType, DocumentNumber: "DOC-G", SubmittedAt: &now,
	}
	if err := kycRepo.Upsert(context.Background(), p); err != nil {
		t.Fatalf("Upsert KYC profile: %v", err)
	}

	const frozenAmount = 30_000
	freezes := []repository.PrizeFreeze{
		{UserID: gatedUser.ID, AmountCents: frozenAmount, Reason: "kyc_pending"},
	}

	if err := repo.DistributePrizesAtomically(context.Background(), q.ID, nil, freezes); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := kycRepo.GetByUserID(context.Background(), gatedUser.ID)
	if !got.BalanceFrozen {
		t.Error("expected balance_frozen=true")
	}
	if got.FrozenAmountCents != frozenAmount {
		t.Errorf("frozen_amount_cents: got %d, want %d", got.FrozenAmountCents, frozenAmount)
	}
}

func TestQuinielaRepository_DistributePrizesAtomically_Idempotency_ConflictOnSecondCall(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	if err := repo.DistributePrizesAtomically(context.Background(), q.ID, nil, nil); err != nil {
		t.Fatalf("first call: %v", err)
	}

	err := repo.DistributePrizesAtomically(context.Background(), q.ID, nil, nil)
	if err == nil {
		t.Fatal("expected conflict error on second distribution, got nil")
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestQuinielaRepository_DistributePrizesAtomically_CreditNotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	owner := seedUser(t)
	q := seedQuiniela(t, owner.ID)
	repo := repository.NewPostgresQuinielaRepository(testDB)

	credits := []repository.PrizeCredit{
		{UserID: 99999, AmountCents: 1000, RefID: int64(q.ID), RefType: "quiniela"},
	}

	err := repo.DistributePrizesAtomically(context.Background(), q.ID, credits, nil)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}
