package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func seedKYCProfile(t *testing.T, userID int) *domain.KYCProfile {
	t.Helper()
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	now := time.Now()
	docType := domain.KYCDocGovID
	p := &domain.KYCProfile{
		UserID:         userID,
		Status:         domain.KYCStatusPending,
		Tier:           domain.KYCTierUnverified,
		FullName:       "Test User",
		Nationality:    "GT",
		DocumentType:   &docType,
		DocumentNumber: "12345678",
		SubmittedAt:    &now,
	}
	if err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf("seedKYCProfile: %v", err)
	}
	return p
}

// ── Upsert ────────────────────────────────────────────────────────────────────

func TestKYCProfileRepository_Upsert_CreatesProfile(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)

	if p.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if p.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestKYCProfileRepository_Upsert_UpsertsSameUser(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p1 := seedKYCProfile(t, u.ID)

	// Resubmit with different document number — should update in place.
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	now := time.Now()
	docType := domain.KYCDocGovID
	p2 := &domain.KYCProfile{
		UserID:         u.ID,
		Status:         domain.KYCStatusPending,
		Tier:           domain.KYCTierUnverified,
		FullName:       "Test User",
		Nationality:    "GT",
		DocumentType:   &docType,
		DocumentNumber: "99999999",
		SubmittedAt:    &now,
	}
	if err := repo.Upsert(context.Background(), p2); err != nil {
		t.Fatalf("Upsert on conflict: %v", err)
	}
	if p2.ID != p1.ID {
		t.Errorf("expected same profile id on upsert, got %d vs %d", p2.ID, p1.ID)
	}
}

// ── GetByUserID ───────────────────────────────────────────────────────────────

func TestKYCProfileRepository_GetByUserID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	created := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	got, err := repo.GetByUserID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("GetByUserID: got %v, want id=%d", got, created.ID)
	}
}

func TestKYCProfileRepository_GetByUserID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	got, err := repo.GetByUserID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestKYCProfileRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	created := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
}

func TestKYCProfileRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

// ── UpdateStatus ──────────────────────────────────────────────────────────────

func TestKYCProfileRepository_UpdateStatus_Transitions(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.UpdateStatus(context.Background(), p.ID, domain.KYCStatusApproved, admin.ID, ""); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.Status != domain.KYCStatusApproved {
		t.Errorf("status: got %q, want approved", got.Status)
	}
}

func TestKYCProfileRepository_UpdateStatus_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.UpdateStatus(context.Background(), 99999, domain.KYCStatusApproved, 1, "")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── UpdateTier ────────────────────────────────────────────────────────────────

func TestKYCProfileRepository_UpdateTier_SetsProfileAndUser(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.UpdateTier(context.Background(), u.ID, domain.KYCTierOne, nil); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByUserID(context.Background(), u.ID)
	if got.Tier != domain.KYCTierOne {
		t.Errorf("tier: got %d, want 1", got.Tier)
	}
}

func TestKYCProfileRepository_UpdateTier_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.UpdateTier(context.Background(), 99999, domain.KYCTierOne, nil); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── SetFrozen ─────────────────────────────────────────────────────────────────

func TestKYCProfileRepository_SetFrozen_FreezesThenUnfreezes(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.SetFrozen(context.Background(), u.ID, true, 50000, "suspected fraud"); err != nil {
		t.Fatalf("SetFrozen(true): %v", err)
	}

	got, _ := repo.GetByUserID(context.Background(), u.ID)
	if !got.BalanceFrozen {
		t.Error("expected balance_frozen=true")
	}
	if got.FrozenAmountCents != 50000 {
		t.Errorf("frozen_amount_cents: got %d, want 50000", got.FrozenAmountCents)
	}

	if err := repo.SetFrozen(context.Background(), u.ID, false, 0, ""); err != nil {
		t.Fatalf("SetFrozen(false): %v", err)
	}

	got2, _ := repo.GetByUserID(context.Background(), u.ID)
	if got2.BalanceFrozen {
		t.Error("expected balance_frozen=false after unfreeze")
	}
}

func TestKYCProfileRepository_SetFrozen_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.SetFrozen(context.Background(), 99999, true, 1000, "test"); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── ListPending ───────────────────────────────────────────────────────────────

func TestKYCProfileRepository_ListPending_ReturnsActiveQueue(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	results, err := repo.ListPending(context.Background(),
		repository.KYCProfileFilters{},
		repository.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 pending profile, got %d", len(results))
	}
}

func TestKYCProfileRepository_ListPending_FilterByStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	_ = repo.UpdateStatus(context.Background(), p.ID, domain.KYCStatusApproved, admin.ID, "")

	st := domain.KYCStatusApproved
	results, err := repo.ListPending(context.Background(),
		repository.KYCProfileFilters{Status: &st},
		repository.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].Status != domain.KYCStatusApproved {
		t.Errorf("expected 1 approved profile, got %d", len(results))
	}
}

// ── ListFrozen ────────────────────────────────────────────────────────────────

func TestKYCProfileRepository_ListFrozen_ReturnsOnlyFrozen(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	_ = repo.SetFrozen(context.Background(), u.ID, true, 10000, "test")

	summaries, err := repo.ListFrozen(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 frozen account, got %d", len(summaries))
	}
	if summaries[0].UserID != u.ID {
		t.Errorf("UserID: got %d, want %d", summaries[0].UserID, u.ID)
	}
}

func TestKYCProfileRepository_ListFrozen_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	summaries, err := repo.ListFrozen(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected empty slice, got %d", len(summaries))
	}
}

// ── ListDueForReview ──────────────────────────────────────────────────────────

func TestKYCProfileRepository_ListDueForReview_ReturnsExpired(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// Approve and set next_review_at in the past.
	_ = repo.UpdateStatus(context.Background(), p.ID, domain.KYCStatusApproved, admin.ID, "")
	past := time.Now().Add(-24 * time.Hour)
	_ = repo.UpdateTier(context.Background(), u.ID, domain.KYCTierOne, &past)

	results, err := repo.ListDueForReview(context.Background(), time.Now())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 due-for-review profile, got %d", len(results))
	}
}

// ── CountReviewQueue ──────────────────────────────────────────────────────────

func TestKYCProfileRepository_CountReviewQueue_ReturnsCount(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	_ = repo.UpdateStatus(context.Background(), p.ID, domain.KYCStatusUnderReview, admin.ID, "")

	n, err := repo.CountReviewQueue(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

// ── SumFrozenAmountCents ──────────────────────────────────────────────────────

func TestKYCProfileRepository_SumFrozenAmountCents_ReturnsSum(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	_ = repo.SetFrozen(context.Background(), u.ID, true, 25000, "test")

	total, err := repo.SumFrozenAmountCents(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if total != 25000 {
		t.Errorf("SumFrozenAmountCents: got %d, want 25000", total)
	}
}

func TestKYCProfileRepository_SumFrozenAmountCents_EmptyIsZero(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	total, err := repo.SumFrozenAmountCents(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if total != 0 {
		t.Errorf("expected 0, got %d", total)
	}
}

// ── CountRecentSubmissionsByIP ────────────────────────────────────────────────

func TestKYCProfileRepository_CountRecentSubmissionsByIP_EmptyIP_ReturnsZero(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	n, err := repo.CountRecentSubmissionsByIP(context.Background(), "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 for empty IP, got %d", n)
	}
}

func TestKYCProfileRepository_CountRecentSubmissionsByIP_CountsWithinWindow(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	ip := "192.168.1.99"
	now := time.Now()
	docType := domain.KYCDocGovID
	// Insert two profiles with the same submission_ip via Upsert.
	p1 := &domain.KYCProfile{
		UserID: u1.ID, Status: domain.KYCStatusPending, Tier: domain.KYCTierUnverified,
		FullName: "Alice", DocumentType: &docType, DocumentNumber: "DOC-A",
		SubmissionIP: &ip, SubmittedAt: &now,
	}
	p2 := &domain.KYCProfile{
		UserID: u2.ID, Status: domain.KYCStatusPending, Tier: domain.KYCTierUnverified,
		FullName: "Bob", DocumentType: &docType, DocumentNumber: "DOC-B",
		SubmissionIP: &ip, SubmittedAt: &now,
	}
	if err := repo.Upsert(context.Background(), p1); err != nil {
		t.Fatalf("Upsert p1: %v", err)
	}
	if err := repo.Upsert(context.Background(), p2); err != nil {
		t.Fatalf("Upsert p2: %v", err)
	}

	since := now.Add(-time.Minute)
	n, err := repo.CountRecentSubmissionsByIP(context.Background(), ip, since)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 2 {
		t.Errorf("expected 2 submissions from same IP within window, got %d", n)
	}
}

func TestKYCProfileRepository_CountRecentSubmissionsByIP_ExcludesOutsideWindow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	ip := "10.0.0.5"
	old := time.Now().Add(-2 * time.Hour)
	docType := domain.KYCDocGovID
	p := &domain.KYCProfile{
		UserID: u.ID, Status: domain.KYCStatusPending, Tier: domain.KYCTierUnverified,
		FullName: "Carlos", DocumentType: &docType, DocumentNumber: "DOC-C",
		SubmissionIP: &ip, SubmittedAt: &old,
	}
	if err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Update submitted_at to a time outside the 1-hour window.
	_, err := testDB.Exec(context.Background(),
		`UPDATE kyc_profiles SET submitted_at = $1 WHERE user_id = $2`,
		old, u.ID)
	if err != nil {
		t.Fatalf("update submitted_at: %v", err)
	}

	since := time.Now().Add(-time.Hour)
	n, err := repo.CountRecentSubmissionsByIP(context.Background(), ip, since)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 for submission outside window, got %d", n)
	}
}

// ── UpdateRiskScore ───────────────────────────────────────────────────────────

func TestKYCProfileRepository_UpdateRiskScore_SetsScore(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	if err := repo.UpdateRiskScore(context.Background(), p.ID, 75); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.RiskScore != 75 {
		t.Errorf("risk_score: got %d, want 75", got.RiskScore)
	}
}

// ── ExistsByDocumentIdentity ──────────────────────────────────────────────────

func TestKYCProfileRepository_ExistsByDocumentIdentity_DetectsDuplicate(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// u1 already has a pending profile with doc 12345678 (from seedKYCProfile).
	seedKYCProfile(t, u1.ID)

	exists, err := repo.ExistsByDocumentIdentity(context.Background(),
		domain.KYCDocGovID, "12345678", nil, u2.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !exists {
		t.Error("expected duplicate to be found")
	}
}

func TestKYCProfileRepository_ExistsByDocumentIdentity_ExcludesSelf(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	exists, err := repo.ExistsByDocumentIdentity(context.Background(),
		domain.KYCDocGovID, "12345678", nil, u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if exists {
		t.Error("expected no duplicate when excluding same user")
	}
}

// ── UpdateStatus with zero reviewerID ────────────────────────────────────────

func TestKYCProfileRepository_UpdateStatus_ZeroReviewerID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// reviewedBy=0 → reviewer pointer stays nil (system transition).
	if err := repo.UpdateStatus(context.Background(), p.ID, domain.KYCStatusUnderReview, 0, ""); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

// ── ListPending with Tier filter ──────────────────────────────────────────────

func TestKYCProfileRepository_ListPending_FilterByTier(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	p1 := seedKYCProfile(t, u1.ID)
	seedKYCProfile(t, u2.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// Promote p1 to tier 1 so we can filter by it.
	_ = repo.UpdateStatus(context.Background(), p1.ID, domain.KYCStatusApproved, admin.ID, "")
	_ = repo.UpdateTier(context.Background(), u1.ID, domain.KYCTierOne, nil)

	tier := domain.KYCTierOne
	st := domain.KYCStatusApproved
	results, err := repo.ListPending(context.Background(),
		repository.KYCProfileFilters{Status: &st, Tier: &tier},
		repository.Pagination{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 tier-1 profile, got %d", len(results))
	}
}

// ── Upsert with nil DocumentType ──────────────────────────────────────────────

func TestKYCProfileRepository_Upsert_NilDocumentType_ScansCorrectly(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// Create profile with no document type — document_type column stays NULL.
	now := time.Now()
	p := &domain.KYCProfile{
		UserID:      u.ID,
		Status:      domain.KYCStatusPending,
		Tier:        domain.KYCTierUnverified,
		FullName:    "No Doc User",
		Nationality: "GT",
		SubmittedAt: &now,
	}
	if err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.GetByUserID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.DocumentType != nil {
		t.Errorf("expected nil DocumentType, got %v", got.DocumentType)
	}
}

// ── RiskDashboardStats ────────────────────────────────────────────────────────

func TestKYCProfileRepository_RiskDashboardStats_ReturnsStats(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	stats, err := repo.RiskDashboardStats(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.TierDistribution == nil {
		t.Error("expected non-nil TierDistribution map")
	}
}

func TestKYCProfileRepository_RiskDashboardStats_WithData(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	stats, err := repo.RiskDashboardStats(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if stats.TierDistribution[domain.KYCTierUnverified] != 1 {
		t.Errorf("TierDistribution[0]: got %d, want 1", stats.TierDistribution[domain.KYCTierUnverified])
	}
}

// ── ReleaseAndCreditFrozen ────────────────────────────────────────────────────

func TestKYCProfileRepository_ReleaseAndCreditFrozen_CreditsBalanceAndClearsFreeze(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)
	userRepo := repository.NewPostgresUserRepository(testDB)

	const frozenCents = 50_000 // Q500
	if err := repo.SetFrozen(context.Background(), u.ID, true, frozenCents, "prize_win"); err != nil {
		t.Fatalf("SetFrozen: %v", err)
	}

	credited, err := repo.ReleaseAndCreditFrozen(context.Background(), u.ID, int64(p.ID), "kyc_unfreeze")
	if err != nil {
		t.Fatalf("ReleaseAndCreditFrozen: %v", err)
	}
	if credited != frozenCents {
		t.Errorf("credited: got %d, want %d", credited, frozenCents)
	}

	// users.balance_cents must have been incremented.
	balanceCents, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balanceCents != frozenCents {
		t.Errorf("balance_cents: got %d, want %d", balanceCents, frozenCents)
	}

	// kyc_profiles freeze columns must be cleared.
	got, _ := repo.GetByUserID(context.Background(), u.ID)
	if got.BalanceFrozen {
		t.Error("expected balance_frozen=false after release")
	}
	if got.FrozenAmountCents != 0 {
		t.Errorf("frozen_amount_cents: got %d, want 0", got.FrozenAmountCents)
	}

	// A balance_ledger row with kind='prize' must exist.
	var count int
	err = testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM balance_ledger WHERE user_id = $1 AND kind = 'prize'`, u.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("balance_ledger query: %v", err)
	}
	if count != 1 {
		t.Errorf("balance_ledger rows: got %d, want 1", count)
	}
}

func TestKYCProfileRepository_ReleaseAndCreditFrozen_NotFrozen_IdempotentNoOp(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	// Profile is not frozen — calling release must return 0, nil.
	credited, err := repo.ReleaseAndCreditFrozen(context.Background(), u.ID, int64(p.ID), "kyc_unfreeze")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credited != 0 {
		t.Errorf("expected 0 credited for non-frozen profile, got %d", credited)
	}
}

// ── UpdateStatusWithEvent ─────────────────────────────────────────────────────

func TestKYCProfileRepository_UpdateStatusWithEvent_TransitionsStatusAndWritesAuditRow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.UpdateStatusWithEvent(
		context.Background(),
		p.ID, admin.ID,
		domain.KYCStatusPending, domain.KYCStatusUnderReview,
		domain.KYCEventUnderReview,
		"", "trace-001",
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.Status != domain.KYCStatusUnderReview {
		t.Errorf("status: got %q, want under_review", got.Status)
	}

	var count int
	err = testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM kyc_events WHERE profile_id = $1 AND event_type = 'under_review'`,
		p.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("kyc_events query: %v", err)
	}
	if count != 1 {
		t.Errorf("kyc_events rows: got %d, want 1", count)
	}
}

func TestKYCProfileRepository_UpdateStatusWithEvent_RejectionWritesReason(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.UpdateStatusWithEvent(
		context.Background(),
		p.ID, admin.ID,
		domain.KYCStatusPending, domain.KYCStatusRejected,
		domain.KYCEventRejected,
		"document expired", "trace-002",
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.Status != domain.KYCStatusRejected {
		t.Errorf("status: got %q, want rejected", got.Status)
	}
}

func TestKYCProfileRepository_UpdateStatusWithEvent_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.UpdateStatusWithEvent(
		context.Background(),
		99999, 1,
		domain.KYCStatusPending, domain.KYCStatusUnderReview,
		domain.KYCEventUnderReview,
		"", "",
	)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── ApproveAndSetTier ─────────────────────────────────────────────────────────

func TestKYCProfileRepository_ApproveAndSetTier_ApprovesAndPropagatesTier(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.ApproveAndSetTier(
		context.Background(),
		p.ID, admin.ID,
		domain.KYCTierOne,
		time.Now().Add(365*24*time.Hour),
		"all clear", "trace-003",
		domain.KYCStatusPending,
	)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), p.ID)
	if got.Status != domain.KYCStatusApproved {
		t.Errorf("status: got %q, want approved", got.Status)
	}
	if got.Tier != domain.KYCTierOne {
		t.Errorf("tier: got %d, want 1", got.Tier)
	}

	// users.kyc_tier must be denormalised.
	userRepo := repository.NewPostgresUserRepository(testDB)
	usr, _ := userRepo.GetByID(context.Background(), u.ID)
	if usr.KYCTier != domain.KYCTierOne {
		t.Errorf("users.kyc_tier: got %d, want 1", usr.KYCTier)
	}

	// Audit event must be present.
	var count int
	err = testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM kyc_events WHERE profile_id = $1 AND event_type = 'approved'`,
		p.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("kyc_events query: %v", err)
	}
	if count != 1 {
		t.Errorf("kyc_events rows: got %d, want 1", count)
	}
}

func TestKYCProfileRepository_ApproveAndSetTier_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.ApproveAndSetTier(
		context.Background(),
		99999, 1,
		domain.KYCTierOne,
		time.Now().Add(365*24*time.Hour),
		"", "",
		domain.KYCStatusPending,
	)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── FreezeAtomic ─────────────────────────────────────────────────────────────

func TestKYCProfileRepository_FreezeAtomic_FreezesAndWritesAuditRow(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	const frozenCents = 75_000
	err := repo.FreezeAtomic(context.Background(), u.ID, frozenCents, "suspected_fraud", "trace-004")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByUserID(context.Background(), u.ID)
	if !got.BalanceFrozen {
		t.Error("expected balance_frozen=true")
	}
	if got.FrozenAmountCents != frozenCents {
		t.Errorf("frozen_amount_cents: got %d, want %d", got.FrozenAmountCents, frozenCents)
	}

	var count int
	err = testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM kyc_events WHERE profile_id = $1 AND event_type = 'frozen'`,
		got.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("kyc_events query: %v", err)
	}
	if count != 1 {
		t.Errorf("kyc_events rows: got %d, want 1", count)
	}
}

func TestKYCProfileRepository_FreezeAtomic_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCProfileRepository(testDB)

	err := repo.FreezeAtomic(context.Background(), 99999, 1000, "test", "trace-005")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}
