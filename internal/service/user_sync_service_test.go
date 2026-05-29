package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// clerkSyncRepo provides per-method error control for ClerkUserSyncer tests.
// Unlike stubUserRepo (ranking_service_test.go), this stub lets GetByClerkSubject,
// Create, Update, and Delete return independent errors.
type clerkSyncRepo struct {
	existingUser *domain.User
	getErr       error
	createErr    error
	updateErr    error
	deleteErr    error
	created      *domain.User // captures the argument passed to Create
	deletedID    int          // captures the id argument passed to Delete
}

func (r *clerkSyncRepo) Create(_ context.Context, u *domain.User) error {
	r.created = u
	return r.createErr
}
func (r *clerkSyncRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, nil
}
func (r *clerkSyncRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.existingUser, r.getErr
}
func (r *clerkSyncRepo) Update(_ context.Context, _ *domain.User) error { return r.updateErr }
func (r *clerkSyncRepo) Delete(_ context.Context, id int) error {
	r.deletedID = id
	return r.deleteErr
}
func (r *clerkSyncRepo) List(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *clerkSyncRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, nil
}
func (r *clerkSyncRepo) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *clerkSyncRepo) Unban(_ context.Context, _ int) error                 { return nil }
func (r *clerkSyncRepo) ListBanned(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *clerkSyncRepo) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", nil
}
func (r *clerkSyncRepo) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, nil
}
func (r *clerkSyncRepo) GetBalance(_ context.Context, _ int) (int, int, error) { return 0, 0, nil }
func (r *clerkSyncRepo) UpdateLocale(_ context.Context, _ int, _ string) error { return nil }

// kycProfileStubForSync is a minimal KYCProfileRepository stub that only
// implements the methods exercised by ClerkUserSyncService (EnsureStub).
// All other methods return zero values so the stub satisfies the interface.
type kycProfileStubForSync struct {
	ensureStubErr    error
	ensureStubCalled bool
}

func (s *kycProfileStubForSync) EnsureStub(_ context.Context, _ int) error {
	s.ensureStubCalled = true
	return s.ensureStubErr
}

// Remaining KYCProfileRepository methods — no-ops for this stub.
func (s *kycProfileStubForSync) Upsert(_ context.Context, _ *domain.KYCProfile) error { return nil }
func (s *kycProfileStubForSync) GetByUserID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) GetByID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
	return nil
}
func (s *kycProfileStubForSync) UpdateStatusWithEvent(_ context.Context, _, _ int, _ repository.KYCStatusEvent) error {
	return nil
}
func (s *kycProfileStubForSync) UpdateTier(_ context.Context, _ int, _ domain.KYCTier, _ *time.Time) error {
	return nil
}
func (s *kycProfileStubForSync) SetFrozen(_ context.Context, _ int, _ bool, _ int, _ string) error {
	return nil
}
func (s *kycProfileStubForSync) FreezeAtomic(_ context.Context, _, _ int, _, _ string) error {
	return nil
}
func (s *kycProfileStubForSync) FreezeAtomicWithTxHook(_ context.Context, _, _ int, _, _ string, _ func(context.Context, pgx.Tx) error) error {
	return nil
}
func (s *kycProfileStubForSync) ListPending(_ context.Context, _ repository.KYCProfileFilters, _ repository.Pagination) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) ListFrozen(_ context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) ListDueForReview(_ context.Context, _ time.Time) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) CountReviewQueue(_ context.Context) (int64, error) { return 0, nil }
func (s *kycProfileStubForSync) SumFrozenAmountCents(_ context.Context) (int64, error) {
	return 0, nil
}
func (s *kycProfileStubForSync) RiskDashboardStats(_ context.Context) (*domain.KYCRiskDashboardStats, error) {
	return nil, nil
}
func (s *kycProfileStubForSync) ExistsByDocumentIdentity(_ context.Context, _ domain.KYCDocumentType, _ string, _ *time.Time, _ int) (bool, error) {
	return false, nil
}
func (s *kycProfileStubForSync) UpdateRiskScore(_ context.Context, _ int, _ int) error { return nil }
func (s *kycProfileStubForSync) CountRecentSubmissionsByIP(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (s *kycProfileStubForSync) ReleaseAndCreditFrozen(_ context.Context, _ int, _ int64, _ string) (int, error) {
	return 0, nil
}
func (s *kycProfileStubForSync) ApproveAndSetTier(_ context.Context, _, _ int, _ repository.KYCApprovalParams) error {
	return nil
}

func newClerkSyncer(repo *clerkSyncRepo) ClerkUserSyncer {
	return NewClerkUserSyncService(repo, &kycProfileStubForSync{}, zap.NewNop())
}

// ── Upsert - happy paths ──────────────────────────────────────────────────────

func TestClerkUserSyncer_Upsert_NewUser_CallsCreate(t *testing.T) {
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "alice@example.com"}}

	if err := svc.Upsert(context.Background(), "user_abc", "Alice", "Smith", "em_1", emails); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected Create to be called; it was not")
	}
	if repo.created.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %q", repo.created.Email)
	}
	if repo.created.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", repo.created.Name)
	}
	if repo.created.Role != domain.RoleUser {
		t.Errorf("expected role %q, got %q", domain.RoleUser, repo.created.Role)
	}
}

func TestClerkUserSyncer_Upsert_ExistingUser_CallsUpdate(t *testing.T) {
	existing := &domain.User{ID: 7, Name: "Old Name", Email: "old@example.com", ClerkSubject: "user_abc"}
	repo := &clerkSyncRepo{existingUser: existing}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_2", Address: "new@example.com"}}

	if err := svc.Upsert(context.Background(), "user_abc", "New", "Name", "em_2", emails); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if existing.Email != "new@example.com" {
		t.Errorf("expected updated email new@example.com, got %q", existing.Email)
	}
	if existing.Name != "New Name" {
		t.Errorf("expected updated name 'New Name', got %q", existing.Name)
	}
}

func TestClerkUserSyncer_Upsert_EmptyName_FallsBackToSubject(t *testing.T) {
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "x@example.com"}}

	if err := svc.Upsert(context.Background(), "user_fallback", "", "", "em_1", emails); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created == nil || repo.created.Name != "user_fallback" {
		name := ""
		if repo.created != nil {
			name = repo.created.Name
		}
		t.Errorf("expected name 'user_fallback', got %q", name)
	}
}

func TestClerkUserSyncer_Upsert_NoEmailAddresses_SkipsValidation(t *testing.T) {
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)

	if err := svc.Upsert(context.Background(), "user_noemail", "A", "B", "", nil); err != nil {
		t.Fatalf("unexpected error for empty email list: %v", err)
	}
}

// ── Upsert - error paths ──────────────────────────────────────────────────────

func TestClerkUserSyncer_Upsert_InvalidEmail_ReturnsValidation(t *testing.T) {
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "notanemail"}}

	err := svc.Upsert(context.Background(), "user_bad", "A", "B", "em_1", emails)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestClerkUserSyncer_Upsert_GetBySubjectError_ReturnsInternal(t *testing.T) {
	repo := &clerkSyncRepo{getErr: errors.New("connection reset")}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "a@b.com"}}

	err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_1", emails)
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Errorf("expected internal error, got %v", err)
	}
}

func TestClerkUserSyncer_Upsert_CreateError_Propagates(t *testing.T) {
	createErr := errors.New("insert failed")
	repo := &clerkSyncRepo{createErr: createErr}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "a@b.com"}}

	if err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_1", emails); err == nil {
		t.Fatal("expected error from Create, got nil")
	}
}

func TestClerkUserSyncer_Upsert_UpdateError_Propagates(t *testing.T) {
	existing := &domain.User{ID: 3, ClerkSubject: "user_x"}
	repo := &clerkSyncRepo{existingUser: existing, updateErr: errors.New("update failed")}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{{ID: "em_1", Address: "a@b.com"}}

	if err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_1", emails); err == nil {
		t.Fatal("expected error from Update, got nil")
	}
}

// ── resolvePrimaryEmail (tested via Upsert) ───────────────────────────────────

func TestClerkUserSyncer_PrimaryEmail_MatchingID_UsesCorrectAddress(t *testing.T) {
	// First address is invalid; primary ID points to second (valid) address.
	// If the wrong address is selected, email validation will fail.
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{
		{ID: "em_first", Address: "notanemail"},
		{ID: "em_primary", Address: "real@example.com"},
	}

	if err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_primary", emails); err != nil {
		t.Fatalf("expected primary email to be resolved; got error: %v", err)
	}
}

func TestClerkUserSyncer_PrimaryEmail_NonMatchingID_FallsBackToFirst(t *testing.T) {
	// primaryEmailID does not match any entry - service should fall back to the first
	// address and log a warning (not fail).
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)
	emails := []ClerkEmail{
		{ID: "em_first", Address: "fallback@example.com"},
		{ID: "em_second", Address: "other@example.com"},
	}

	if err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_nonexistent", emails); err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
}

func TestClerkUserSyncer_PrimaryEmail_EmptyList_EmptyEmail(t *testing.T) {
	repo := &clerkSyncRepo{}
	svc := newClerkSyncer(repo)

	// Empty address list with a non-empty primaryEmailID - should not error.
	if err := svc.Upsert(context.Background(), "user_x", "A", "B", "em_ghost", nil); err != nil {
		t.Fatalf("expected no error for empty address list: %v", err)
	}
}

// ── SoftDelete ────────────────────────────────────────────────────────────────

// TestClerkUserSyncer_SoftDelete_ExistingUser_CallsDeleteByID verifies that
// SoftDelete resolves the Clerk subject to an internal user ID and delegates
// to userRepo.Delete with that ID.
func TestClerkUserSyncer_SoftDelete_ExistingUser_CallsDeleteByID(t *testing.T) {
	existing := &domain.User{ID: 42, ClerkSubject: "user_del"}
	repo := &clerkSyncRepo{existingUser: existing}
	svc := newClerkSyncer(repo)

	if err := svc.SoftDelete(context.Background(), "user_del"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedID != 42 {
		t.Errorf("expected Delete called with id=42, got id=%d", repo.deletedID)
	}
}

// TestClerkUserSyncer_SoftDelete_UserNotFound_IsNoOp verifies idempotency:
// when no active row exists for the subject (already deleted or never synced)
// the call succeeds without invoking Delete.
func TestClerkUserSyncer_SoftDelete_UserNotFound_IsNoOp(t *testing.T) {
	repo := &clerkSyncRepo{existingUser: nil}
	svc := newClerkSyncer(repo)

	if err := svc.SoftDelete(context.Background(), "user_already_gone"); err != nil {
		t.Fatalf("expected no-op for unknown subject, got error: %v", err)
	}
	if repo.deletedID != 0 {
		t.Errorf("expected Delete not to be called, but deletedID=%d", repo.deletedID)
	}
}

// TestClerkUserSyncer_SoftDelete_GetBySubjectError_ReturnsInternal verifies
// that a DB error during subject lookup is surfaced as an internal error so
// the webhook handler returns 500 and Clerk retries the delivery.
func TestClerkUserSyncer_SoftDelete_GetBySubjectError_ReturnsInternal(t *testing.T) {
	repo := &clerkSyncRepo{getErr: errors.New("connection reset")}
	svc := newClerkSyncer(repo)

	err := svc.SoftDelete(context.Background(), "user_x")
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Errorf("expected internal error, got %v", err)
	}
}

// TestClerkUserSyncer_SoftDelete_DeleteError_Propagates verifies that a DB
// error during the Delete call is surfaced so Clerk retries the delivery.
func TestClerkUserSyncer_SoftDelete_DeleteError_Propagates(t *testing.T) {
	existing := &domain.User{ID: 7, ClerkSubject: "user_x"}
	repo := &clerkSyncRepo{existingUser: existing, deleteErr: errors.New("update failed")}
	svc := newClerkSyncer(repo)

	if err := svc.SoftDelete(context.Background(), "user_x"); err == nil {
		t.Fatal("expected error from Delete, got nil")
	}
}

// ── KYC profile stub creation ─────────────────────────────────────────────────

// TestClerkUserSyncer_Upsert_NewUser_CreatesKYCProfileStub verifies that
// EnsureStub is called after a new user is created, so that every user is
// guaranteed to have a kyc_profiles row for prize distribution.
func TestClerkUserSyncer_Upsert_NewUser_CreatesKYCProfileStub(t *testing.T) {
	repo := &clerkSyncRepo{}
	kycRepo := &kycProfileStubForSync{}
	svc := NewClerkUserSyncService(repo, kycRepo, zap.NewNop())
	emails := []ClerkEmail{{ID: "em_1", Address: "stub@example.com"}}

	if err := svc.Upsert(context.Background(), "user_stub", "Stub", "User", "em_1", emails); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !kycRepo.ensureStubCalled {
		t.Error("expected EnsureStub to be called for new user; it was not")
	}
}

// TestClerkUserSyncer_Upsert_ExistingUser_DoesNotCreateKYCProfileStub verifies
// that EnsureStub is NOT called for an update (existing user), because the stub
// was already created at their original registration.
func TestClerkUserSyncer_Upsert_ExistingUser_DoesNotCreateKYCProfileStub(t *testing.T) {
	existing := &domain.User{ID: 11, ClerkSubject: "user_existing"}
	repo := &clerkSyncRepo{existingUser: existing}
	kycRepo := &kycProfileStubForSync{}
	svc := NewClerkUserSyncService(repo, kycRepo, zap.NewNop())
	emails := []ClerkEmail{{ID: "em_1", Address: "existing@example.com"}}

	if err := svc.Upsert(context.Background(), "user_existing", "Existing", "User", "em_1", emails); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kycRepo.ensureStubCalled {
		t.Error("EnsureStub should not be called for an existing user update")
	}
}

// TestClerkUserSyncer_Upsert_EnsureStubError_IsLogged verifies that a transient
// EnsureStub error does not surface as a Upsert error — the user row is already
// committed and the backfill migration covers recovery.
func TestClerkUserSyncer_Upsert_EnsureStubError_IsLogged(t *testing.T) {
	repo := &clerkSyncRepo{}
	kycRepo := &kycProfileStubForSync{ensureStubErr: errors.New("db timeout")}
	svc := NewClerkUserSyncService(repo, kycRepo, zap.NewNop())
	emails := []ClerkEmail{{ID: "em_1", Address: "erruser@example.com"}}

	// Must succeed even when EnsureStub fails — user creation committed.
	if err := svc.Upsert(context.Background(), "user_err", "Err", "User", "em_1", emails); err != nil {
		t.Fatalf("EnsureStub failure should not surface as Upsert error; got: %v", err)
	}
}
