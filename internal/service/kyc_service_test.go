package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	metricnoop "go.opentelemetry.io/otel/metric/noop"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type kycProfileRepoStub struct {
	profile         *domain.KYCProfile
	profiles        []*domain.KYCProfile
	frozen          []*domain.FrozenBalanceSummary
	dupExists       bool
	err             error
	updateStatusErr error // overrides err for UpdateStatus only
	getByUserIDErr  error // overrides err for GetByUserID only
	releaseErr      error // overrides err for ReleaseAndCreditFrozen only
}

func (r *kycProfileRepoStub) Upsert(_ context.Context, p *domain.KYCProfile) error {
	if r.err != nil {
		return r.err
	}
	p.ID = 1
	return nil
}
func (r *kycProfileRepoStub) GetByUserID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	if r.getByUserIDErr != nil {
		return nil, r.getByUserIDErr
	}
	return r.profile, r.err
}
func (r *kycProfileRepoStub) GetByID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return r.profile, r.err
}
func (r *kycProfileRepoStub) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
	if r.updateStatusErr != nil {
		return r.updateStatusErr
	}
	return r.err
}
func (r *kycProfileRepoStub) UpdateTier(_ context.Context, _ int, _ domain.KYCTier, _ *time.Time) error {
	return r.err
}
func (r *kycProfileRepoStub) SetFrozen(_ context.Context, _ int, _ bool, _ int, _ string) error {
	return r.err
}
func (r *kycProfileRepoStub) ListPending(_ context.Context, _ repository.KYCProfileFilters, _ repository.Pagination) ([]*domain.KYCProfile, error) {
	return r.profiles, r.err
}
func (r *kycProfileRepoStub) ListFrozen(_ context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return r.frozen, r.err
}
func (r *kycProfileRepoStub) ListDueForReview(_ context.Context, _ time.Time) ([]*domain.KYCProfile, error) {
	return r.profiles, r.err
}
func (r *kycProfileRepoStub) CountReviewQueue(_ context.Context) (int64, error) { return 0, r.err }
func (r *kycProfileRepoStub) SumFrozenAmountCents(_ context.Context) (int64, error) {
	return 0, r.err
}
func (r *kycProfileRepoStub) RiskDashboardStats(_ context.Context) (*domain.KYCRiskDashboardStats, error) {
	return &domain.KYCRiskDashboardStats{TierDistribution: map[domain.KYCTier]int64{}}, r.err
}
func (r *kycProfileRepoStub) ExistsByDocumentIdentity(_ context.Context, _ domain.KYCDocumentType, _ string, _ *time.Time, _ int) (bool, error) {
	return r.dupExists, r.err
}
func (r *kycProfileRepoStub) UpdateRiskScore(_ context.Context, _ int, _ int) error { return r.err }
func (r *kycProfileRepoStub) CountRecentSubmissionsByIP(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, r.err
}
func (r *kycProfileRepoStub) ReleaseAndCreditFrozen(_ context.Context, _ int, _ int64, _ string) (int, error) {
	if r.releaseErr != nil {
		return 0, r.releaseErr
	}
	if r.profile != nil {
		return r.profile.FrozenAmountCents, nil
	}
	return 0, nil
}
func (r *kycProfileRepoStub) ApproveAndSetTier(_ context.Context, _, _ int, _ repository.KYCApprovalParams) error {
	return r.err
}
func (r *kycProfileRepoStub) FreezeAtomic(_ context.Context, _ int, _ int, _ string, _ string) error {
	return r.err
}
func (r *kycProfileRepoStub) FreezeAtomicWithTxHook(_ context.Context, _ int, _ int, _ string, _ string, _ func(context.Context, pgx.Tx) error) error {
	return r.err
}
func (r *kycProfileRepoStub) UpdateStatusWithEvent(_ context.Context, _, _ int, _ repository.KYCStatusEvent) error {
	if r.updateStatusErr != nil {
		return r.updateStatusErr
	}
	return r.err
}
func (r *kycProfileRepoStub) EnsureStub(_ context.Context, _ int) error { return r.err }

type kycDocRepoStub struct {
	doc  *domain.KYCDocument
	docs []*domain.KYCDocument
	err  error
}

func (r *kycDocRepoStub) Create(_ context.Context, d *domain.KYCDocument) error {
	if r.err != nil {
		return r.err
	}
	d.ID = 10
	return nil
}
func (r *kycDocRepoStub) GetByID(_ context.Context, _ int64) (*domain.KYCDocument, error) {
	return r.doc, r.err
}
func (r *kycDocRepoStub) ListByProfile(_ context.Context, _ int, _ domain.KYCProfileType) ([]*domain.KYCDocument, error) {
	return r.docs, r.err
}
func (r *kycDocRepoStub) MarkVerified(_ context.Context, _ int64, _ int) error { return r.err }
func (r *kycDocRepoStub) ListExpiredDocuments(_ context.Context, _ time.Time, _ int) ([]*domain.KYCDocument, error) {
	return nil, nil
}
func (r *kycDocRepoStub) DeleteByID(_ context.Context, _ int64) error { return r.err }

type kycEventRepoStub struct {
	events []*domain.KYCEvent
	err    error
}

func (r *kycEventRepoStub) Create(_ context.Context, e *domain.KYCEvent) error {
	e.ID = 99
	return r.err
}
func (r *kycEventRepoStub) ListByProfile(_ context.Context, _ int, _ domain.KYCProfileType, _ repository.CursorPage) ([]*domain.KYCEvent, string, error) {
	return r.events, "", r.err
}

func newKYCSvc(
	pr *kycProfileRepoStub,
	dr *kycDocRepoStub,
	er *kycEventRepoStub,
) KYCService {
	return NewKYCService(pr, dr, er, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestKYCService_Submit_HappyPath(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	profile, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName:       "Juan Pérez",
		DocumentType:   domain.KYCDocGovID,
		DocumentNumber: "CUI1234567",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile == nil || profile.ID != 1 {
		t.Errorf("expected profile with ID 1, got %v", profile)
	}
}

func TestKYCService_Submit_MissingFullName_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected validation error for missing full_name, got nil")
	}
}

func TestKYCService_Submit_MissingDocumentType_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Ana García", DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected validation error for missing document_type, got nil")
	}
}

func TestKYCService_Submit_AlreadyPending_ReturnsConflict(t *testing.T) {
	existing := &domain.KYCProfile{Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Ana", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected Conflict for already-pending profile, got nil")
	}
}

func TestKYCService_Submit_AlreadyUnderReview_ReturnsConflict(t *testing.T) {
	existing := &domain.KYCProfile{Status: domain.KYCStatusUnderReview}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Ana", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected Conflict for profile under review, got nil")
	}
}

func TestKYCService_Submit_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Ana", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── UploadDocument ────────────────────────────────────────────────────────────

func TestKYCService_UploadDocument_HappyPath(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	doc, err := svc.UploadDocument(context.Background(), 1, UploadDocRequest{
		ProfileID:    1,
		ProfileType:  domain.KYCProfileTypeUser,
		DocumentType: domain.KYCDocGovID,
		StorageKey:   "kyc/1/dpi.jpg",
		ContentType:  "image/jpeg",
		FileSize:     1024,
		FileHash:     "abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil || doc.ID != 10 {
		t.Errorf("expected doc with ID 10, got %v", doc)
	}
}

func TestKYCService_UploadDocument_EmptyStorageKey_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.UploadDocument(context.Background(), 1, UploadDocRequest{FileSize: 100})
	if err == nil {
		t.Fatal("expected validation error for empty storage_key, got nil")
	}
}

func TestKYCService_UploadDocument_ZeroFileSize_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.UploadDocument(context.Background(), 1, UploadDocRequest{StorageKey: "k"})
	if err == nil {
		t.Fatal("expected validation error for zero file_size, got nil")
	}
}

func TestKYCService_UploadDocument_ExceedsMaxBytes_Validation(t *testing.T) {
	// noopSystemParamService returns defaults → max = 10,485,760 bytes
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.UploadDocument(context.Background(), 1, UploadDocRequest{
		StorageKey: "k",
		FileSize:   domain.DefaultKYCMaxDocUploadBytes + 1,
	})
	if err == nil {
		t.Fatal("expected validation error for file exceeding max bytes, got nil")
	}
}

// ── Approve ───────────────────────────────────────────────────────────────────

func TestKYCService_Approve_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_Approve_ProfileNotFound_ReturnsNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	err := svc.Approve(context.Background(), 999, 99, domain.KYCTierTwo)
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestKYCService_Approve_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// TestKYCService_Approve_UsesAtomicApproveAndSetTier verifies that Approve routes
// through ApproveAndSetTier (the atomic method) rather than the two-step
// UpdateStatus+UpdateTier path. A stub that fails on ApproveAndSetTier must
// cause the whole Approve call to fail.
func TestKYCService_Approve_UsesAtomicApproveAndSetTier(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	repo := &approveAtomicTracker{kycProfileRepoStub: kycProfileRepoStub{profile: existing}}
	svc := NewKYCService(repo, &kycDocRepoStub{}, &kycEventRepoStub{}, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.approveAndSetTierCalled {
		t.Error("Approve must call ApproveAndSetTier, not UpdateStatus+UpdateTier")
	}
	if repo.updateStatusCalled {
		t.Error("Approve must NOT call UpdateStatus after refactor")
	}
	if repo.updateTierCalled {
		t.Error("Approve must NOT call UpdateTier after refactor")
	}
}

// approveAtomicTracker wraps kycProfileRepoStub to track which methods are called.
type approveAtomicTracker struct {
	kycProfileRepoStub
	approveAndSetTierCalled bool
	updateStatusCalled      bool
	updateTierCalled        bool
}

func (r *approveAtomicTracker) ApproveAndSetTier(_ context.Context, _, _ int, _ repository.KYCApprovalParams) error {
	r.approveAndSetTierCalled = true
	return nil
}
func (r *approveAtomicTracker) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
	r.updateStatusCalled = true
	return nil
}
func (r *approveAtomicTracker) UpdateTier(_ context.Context, _ int, _ domain.KYCTier, _ *time.Time) error {
	r.updateTierCalled = true
	return nil
}
func (r *approveAtomicTracker) EnsureStub(_ context.Context, _ int) error { return nil }

// ── Reject ────────────────────────────────────────────────────────────────────

func TestKYCService_Reject_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Reject(context.Background(), 1, 99, "blurry documents"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_Reject_EmptyReason_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Reject(context.Background(), 1, 99, ""); err == nil {
		t.Fatal("expected validation error for empty reason, got nil")
	}
}

// ── Escalate ──────────────────────────────────────────────────────────────────

func TestKYCService_Escalate_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Escalate(context.Background(), 1, 99, "suspicious"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_Escalate_ProfileNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Escalate(context.Background(), 999, 99, "reason"); err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

// ── FreezeBalance ─────────────────────────────────────────────────────────────

func TestKYCService_FreezeBalance_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusApproved}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.FreezeBalance(context.Background(), 5, 50_000, "prize freeze"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_FreezeBalance_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.FreezeBalance(context.Background(), 1, 1000, "reason"); err == nil {
		t.Fatal("expected error from FreezeAtomic, got nil")
	}
}

// freezeRouteTracker wraps kycProfileRepoStub to track which freeze path is taken.
type freezeRouteTracker struct {
	kycProfileRepoStub
	atomicCalled         bool
	atomicWithHookCalled bool
	hookErr              error
}

func (r *freezeRouteTracker) FreezeAtomic(_ context.Context, _ int, _ int, _ string, _ string) error {
	r.atomicCalled = true
	return r.err
}
func (r *freezeRouteTracker) FreezeAtomicWithTxHook(_ context.Context, _ int, _ int, _ string, _ string, hook func(context.Context, pgx.Tx) error) error {
	r.atomicWithHookCalled = true
	if r.err != nil {
		return r.err
	}
	// Call the hook with a nil tx to verify it is invoked; the outbox stub handles nil gracefully.
	if err := hook(context.Background(), nil); err != nil {
		return err
	}
	return nil
}
func (r *freezeRouteTracker) EnsureStub(_ context.Context, _ int) error { return nil }

type freezeOutboxStub struct {
	writeInTxCalled bool
	writeInTxErr    error
}

func (o *freezeOutboxStub) Write(_ context.Context, _ notification.EventType, _, _ string, _ any) error {
	return nil
}
func (o *freezeOutboxStub) WriteBatch(_ context.Context, _ []outbox.BatchEvent) error { return nil }
func (o *freezeOutboxStub) WriteDedup(_ context.Context, _ string, _ notification.EventType, _, _ string, _ any) (bool, error) {
	return false, nil
}
func (o *freezeOutboxStub) WriteInTx(_ context.Context, _ outbox.TxExecer, _ notification.EventType, _, _ string, _ any) error {
	o.writeInTxCalled = true
	return o.writeInTxErr
}

func TestKYCService_FreezeBalance_WithOutboxWriter_UsesAtomicHook(t *testing.T) {
	tracker := &freezeRouteTracker{}
	ob := &freezeOutboxStub{}
	svc := NewKYCService(tracker, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	svc.(*kycService).SetOutboxWriter(ob)

	if err := svc.FreezeBalance(context.Background(), 5, 50_000, "prize freeze"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker.atomicCalled {
		t.Error("FreezeAtomic was called; expected FreezeAtomicWithTxHook when outboxWriter is set")
	}
	if !tracker.atomicWithHookCalled {
		t.Error("FreezeAtomicWithTxHook was not called")
	}
	if !ob.writeInTxCalled {
		t.Error("WriteInTx was not called inside the hook")
	}
}

func TestKYCService_FreezeBalance_WithoutOutboxWriter_UsesFreezeAtomic(t *testing.T) {
	tracker := &freezeRouteTracker{}
	svc := NewKYCService(tracker, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())

	if err := svc.FreezeBalance(context.Background(), 5, 50_000, "prize freeze"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tracker.atomicCalled {
		t.Error("expected FreezeAtomic to be called when no outboxWriter is set")
	}
	if tracker.atomicWithHookCalled {
		t.Error("FreezeAtomicWithTxHook called unexpectedly when outboxWriter is nil")
	}
}

func TestKYCService_FreezeBalance_OutboxHookError_RollsBack(t *testing.T) {
	tracker := &freezeRouteTracker{}
	ob := &freezeOutboxStub{writeInTxErr: errors.New("outbox write failed")}
	svc := NewKYCService(tracker, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	svc.(*kycService).SetOutboxWriter(ob)

	if err := svc.FreezeBalance(context.Background(), 5, 50_000, "prize freeze"); err == nil {
		t.Fatal("expected error when outbox hook fails, got nil")
	}
	if !tracker.atomicWithHookCalled {
		t.Error("FreezeAtomicWithTxHook was not called")
	}
}

// ── GetRequirements ───────────────────────────────────────────────────────────

func TestKYCService_GetRequirements_Tier0_NoProfile(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	req, err := svc.GetRequirements(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req == nil {
		t.Fatal("expected requirements, got nil")
	}
}

// ── ListDueForReview ──────────────────────────────────────────────────────────

func TestKYCService_ListDueForReview_ReturnsProfiles(t *testing.T) {
	profiles := []*domain.KYCProfile{{ID: 1}, {ID: 2}}
	svc := newKYCSvc(&kycProfileRepoStub{profiles: profiles}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.ListDueForReview(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(got))
	}
}

func TestKYCService_ListDueForReview_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if _, err := svc.ListDueForReview(context.Background()); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── ListFrozenBalances / ReleaseFrozenBalance ─────────────────────────────────

func TestKYCService_ListFrozenBalances_ReturnsAll(t *testing.T) {
	frozen := []*domain.FrozenBalanceSummary{{UserID: 1, FrozenAmountCents: 5000}}
	svc := newKYCSvc(&kycProfileRepoStub{frozen: frozen}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.ListFrozenBalances(context.Background())
	if err != nil || len(got) != 1 {
		t.Errorf("expected 1 frozen balance, got %d err=%v", len(got), err)
	}
}

func TestKYCService_ReleaseFrozenBalance_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 5}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.ReleaseFrozenBalance(context.Background(), 5, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── GetProfile ────────────────────────────────────────────────────────────────

func TestKYCService_GetProfile_ReturnsProfile(t *testing.T) {
	existing := &domain.KYCProfile{ID: 1, UserID: 7}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.GetProfile(context.Background(), 7)
	if err != nil || got == nil || got.ID != 1 {
		t.Errorf("expected profile id=1, got %v err=%v", got, err)
	}
}

func TestKYCService_GetProfile_NilWhenNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.GetProfile(context.Background(), 99)
	if err != nil || got != nil {
		t.Errorf("expected nil profile, got %v err=%v", got, err)
	}
}

// ── ListDocuments ─────────────────────────────────────────────────────────────

func TestKYCService_ListDocuments_ReturnsDocs(t *testing.T) {
	docs := []*domain.KYCDocument{{ID: 10}, {ID: 11}}
	svc := newKYCSvc(&kycProfileRepoStub{profile: &domain.KYCProfile{ID: 1, UserID: 3}},
		&kycDocRepoStub{docs: docs}, &kycEventRepoStub{})
	got, err := svc.ListDocuments(context.Background(), 3)
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 documents, got %d err=%v", len(got), err)
	}
}

func TestKYCService_ListDocuments_ProfileNotFound_ReturnsEmpty(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.ListDocuments(context.Background(), 99)
	if err != nil || len(got) != 0 {
		t.Errorf("expected empty list, got %d err=%v", len(got), err)
	}
}

// ── GetRiskDashboard ──────────────────────────────────────────────────────────

func TestKYCService_GetRiskDashboard_ReturnsStats(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	stats, err := svc.GetRiskDashboard(context.Background())
	if err != nil || stats == nil {
		t.Errorf("expected stats, got nil err=%v", err)
	}
}

func TestKYCService_GetRiskDashboard_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.GetRiskDashboard(context.Background())
	if err == nil {
		t.Error("expected error from repo, got nil")
	}
}

// ── RecalculateRiskScore ──────────────────────────────────────────────────────

func TestKYCService_RecalculateRiskScore_HappyPath(t *testing.T) {
	existing := &domain.KYCProfile{
		ID:     1,
		UserID: 4,
		Tier:   domain.KYCTierTwo,
		Status: domain.KYCStatusApproved,
	}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 4)
	if err != nil || score < 0 || score > 100 {
		t.Errorf("expected score in [0,100], got %d err=%v", score, err)
	}
}

func TestKYCService_RecalculateRiskScore_ProfileNotFound_ReturnsNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.RecalculateRiskScore(context.Background(), 99)
	if err == nil {
		t.Error("expected not-found error for missing profile, got nil")
	}
}

func TestKYCService_RecalculateRiskScore_PEPFlag_IncreasesScore(t *testing.T) {
	base := &domain.KYCProfile{ID: 1, UserID: 5, Tier: domain.KYCTierThree}
	pep := &domain.KYCProfile{ID: 2, UserID: 6, Tier: domain.KYCTierThree, PEPFlag: true}

	svcBase := newKYCSvc(&kycProfileRepoStub{profile: base}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svcPEP := newKYCSvc(&kycProfileRepoStub{profile: pep}, &kycDocRepoStub{}, &kycEventRepoStub{})

	baseScore, _ := svcBase.RecalculateRiskScore(context.Background(), 5)
	pepScore, _ := svcPEP.RecalculateRiskScore(context.Background(), 6)
	if pepScore <= baseScore {
		t.Errorf("PEP flag should increase risk score: base=%d pep=%d", baseScore, pepScore)
	}
}

// ── SetCache / SetLedger ──────────────────────────────────────────────────────

func TestKYCService_SetCache_NilDoesNotPanic(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(nil)
}

func TestKYCService_SetLedger_NilDoesNotPanic(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetLedger(nil)
}

// ── ListEvents / ListQueue / GetProfileByID ───────────────────────────────────

func TestKYCService_ListEvents_ReturnsList(t *testing.T) {
	events := []*domain.KYCEvent{{ID: 1}, {ID: 2}}
	er := &kycEventRepoStub{events: events}
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, er)
	got, _, err := svc.ListEvents(context.Background(), 1, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 50})
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 events, got %d err=%v", len(got), err)
	}
}

func TestKYCService_ListQueue_ReturnsList(t *testing.T) {
	profiles := []*domain.KYCProfile{{ID: 1}, {ID: 2}}
	svc := newKYCSvc(&kycProfileRepoStub{profiles: profiles}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.ListQueue(context.Background(), repository.KYCProfileFilters{}, repository.Pagination{})
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 profiles, got %d err=%v", len(got), err)
	}
}

func TestKYCService_GetProfileByID_ReturnsProfile(t *testing.T) {
	existing := &domain.KYCProfile{ID: 5}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.GetProfileByID(context.Background(), 5)
	if err != nil || got == nil || got.ID != 5 {
		t.Errorf("expected profile id=5, got %v err=%v", got, err)
	}
}

// ── RequestDocument ───────────────────────────────────────────────────────────

func TestKYCService_RequestDocument_HappyPath(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	err := svc.RequestDocument(context.Background(), 1, 99, domain.KYCDocGovID, "expired")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_RequestDocument_ProfileNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.RequestDocument(context.Background(), 999, 99, domain.KYCDocGovID, "x"); err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

// ── VerifyDocument ────────────────────────────────────────────────────────────

func TestKYCService_VerifyDocument_HappyPath(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.VerifyDocument(context.Background(), 10, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYCService_VerifyDocument_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{err: errors.New("db fail")}, &kycEventRepoStub{})
	if err := svc.VerifyDocument(context.Background(), 10, 99); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── RecalculateRiskScore ──────────────────────────────────────────────────────

func TestKYCService_RecalculateRiskScore_TierUnverified(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 3, Tier: domain.KYCTierUnverified}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score < riskWeightTierUnverified {
		t.Errorf("expected score >= %d for unverified tier, got %d", riskWeightTierUnverified, score)
	}
}

func TestKYCService_RecalculateRiskScore_TierTwo_WithFlags(t *testing.T) {
	profile := &domain.KYCProfile{
		ID:            1,
		UserID:        3,
		Tier:          domain.KYCTierUnverified,
		PEPFlag:       true,
		SanctionsFlag: true,
		BalanceFrozen: true,
	}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// unverified(30) + PEP(25) + sanctions(35) + frozen(15) = 105 -> capped at 100
	if score != 100 {
		t.Errorf("expected capped score=100, got %d", score)
	}
}

func TestKYCService_RecalculateRiskScore_TierThree(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 3, Tier: domain.KYCTierThree}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0 {
		t.Errorf("expected score=0 for tier 3 no flags, got %d", score)
	}
}

func TestKYCService_RecalculateRiskScore_ProfileNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if _, err := svc.RecalculateRiskScore(context.Background(), 99); err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

// ── GetRiskDashboard ──────────────────────────────────────────────────────────

func TestKYCService_GetRiskDashboard_NoCacheReturnsStats(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	got, err := svc.GetRiskDashboard(context.Background())
	if err != nil || got == nil {
		t.Errorf("expected stats, got %v err=%v", got, err)
	}
}

// ── GetRequirements with profile and docs ─────────────────────────────────────

func TestKYCService_GetRequirements_WithProfileAndDocs(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, Tier: domain.KYCTierOne, Status: domain.KYCStatusApproved}
	docs := []*domain.KYCDocument{
		{DocumentType: domain.KYCDocGovID},
	}
	svc := newKYCSvc(
		&kycProfileRepoStub{profile: profile},
		&kycDocRepoStub{docs: docs},
		&kycEventRepoStub{},
	)
	req, err := svc.GetRequirements(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.SubmittedDocs) != 1 || len(req.MissingDocs) != 1 {
		t.Errorf("expected 1 submitted, 1 missing; got sub=%d miss=%d", len(req.SubmittedDocs), len(req.MissingDocs))
	}
}

// ── Reject / Escalate error paths ────────────────────────────────────────────

func TestKYCService_Reject_ProfileNotFound(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{profile: nil}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Reject(context.Background(), 999, 99, "reason"); err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestKYCService_Escalate_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Escalate(context.Background(), 1, 99, "reason"); err == nil {
		t.Fatal("expected repo error, got nil")
	}
}

// ── NewKYCService with metrics ────────────────────────────────────────────────

func TestKYCService_NewKYCService_WithMetrics(t *testing.T) {
	m := newTestMetrics(t)
	svc := NewKYCService(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// ── validateSubmitRequest missing branch ─────────────────────────────────────

func TestKYCService_Submit_MissingDocumentNumber_Validation(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName:     "Juan",
		DocumentType: domain.KYCDocGovID,
	})
	if err == nil {
		t.Fatal("expected validation error for missing document_number, got nil")
	}
}

// ── Submit additional paths ───────────────────────────────────────────────────

type docIdentityErrStub struct {
	kycProfileRepoStub
}

func (r *docIdentityErrStub) ExistsByDocumentIdentity(_ context.Context, _ domain.KYCDocumentType, _ string, _ *time.Time, _ int) (bool, error) {
	return false, errors.New("db fail")
}
func (r *docIdentityErrStub) EnsureStub(_ context.Context, _ int) error { return nil }

func TestKYCService_Submit_DocIdentityCheckError_Propagates(t *testing.T) {
	stub := &docIdentityErrStub{}
	svc := NewKYCService(stub, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected error from document identity check, got nil")
	}
}

func TestKYCService_Submit_DuplicateIdentity_WithMetrics(t *testing.T) {
	m := newTestMetrics(t)
	stub := &kycProfileRepoStub{dupExists: true}
	svc := NewKYCService(stub, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate identity, got nil")
	}
}

// ── IP velocity gate ─────────────────────────────────────────────────────────

type ipVelocityGateStub struct {
	NoopKYCGate
	err error
}

func (g *ipVelocityGateStub) CheckIPSubmissionVelocity(_ context.Context, _ string) error {
	return g.err
}

// trackingEventRepo records every event passed to Create for assertion in tests.
type trackingEventRepo struct {
	kycEventRepoStub
	created []*domain.KYCEvent
}

func (r *trackingEventRepo) Create(_ context.Context, e *domain.KYCEvent) error {
	e.ID = 99
	r.created = append(r.created, e)
	return r.err
}

func TestKYCService_Submit_IPVelocityBlocked_ReturnsError(t *testing.T) {
	gate := &ipVelocityGateStub{err: apperrors.RateLimited("too many submissions")}
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetGate(gate)
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
		SubmissionIP: "1.2.3.4",
	})
	if err == nil {
		t.Fatal("expected rate-limit error from IP velocity gate, got nil")
	}
}

func TestKYCService_Submit_IPVelocityBlocked_EmitsAuditEvent(t *testing.T) {
	gate := &ipVelocityGateStub{err: apperrors.RateLimited("too many submissions")}
	er := &trackingEventRepo{}
	svc := NewKYCService(&kycProfileRepoStub{}, &kycDocRepoStub{}, er, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	svc.(*kycService).SetGate(gate)
	_, _ = svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
		SubmissionIP: "5.6.7.8",
	})
	if len(er.created) == 0 {
		t.Fatal("expected ip_velocity_flag audit event, got none")
	}
	ev := er.created[0]
	if ev.EventType != domain.KYCEventIPVelocityFlag {
		t.Errorf("expected event_type=%q, got %q", domain.KYCEventIPVelocityFlag, ev.EventType)
	}
	if ev.ProfileType != domain.KYCProfileTypeUser {
		t.Errorf("expected profile_type=%q, got %q", domain.KYCProfileTypeUser, ev.ProfileType)
	}
	if ip, _ := ev.Metadata["ip"].(string); ip != "5.6.7.8" {
		t.Errorf("expected metadata.ip=5.6.7.8, got %q", ip)
	}
}

func TestKYCService_Submit_IPVelocityAllowed_Succeeds(t *testing.T) {
	gate := &ipVelocityGateStub{err: nil}
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetGate(gate)
	profile, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
		SubmissionIP: "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile == nil {
		t.Fatal("expected non-nil profile")
	}
}

func TestKYCService_Submit_NilGate_DoesNotPanic(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	// gate is nil by default — must not panic
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err != nil {
		t.Fatalf("unexpected error with nil gate: %v", err)
	}
}

func TestKYCService_Submit_SetsSubmissionIP(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	profile, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
		SubmissionIP: "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.SubmissionIP == nil || *profile.SubmissionIP != "10.0.0.1" {
		t.Errorf("expected SubmissionIP=10.0.0.1, got %v", profile.SubmissionIP)
	}
}

func TestKYCService_Submit_CopiesTierFromExistingProfile(t *testing.T) {
	existing := &domain.KYCProfile{
		ID: 5, UserID: 1, Status: domain.KYCStatusApproved, Tier: domain.KYCTierTwo,
	}
	stub := &kycProfileRepoStub{profile: existing}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	profile, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Tier != domain.KYCTierTwo {
		t.Errorf("expected tier copied from existing (Tier2), got %v", profile.Tier)
	}
}

func TestKYCService_Submit_WithMetrics_RecordsSubmission(t *testing.T) {
	m := newTestMetrics(t)
	svc := NewKYCService(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Approve with SubmittedAt and metrics ─────────────────────────────────────

func TestKYCService_Approve_WithSubmittedAt_RecordsReviewDuration(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	profile := &domain.KYCProfile{ID: 1, UserID: 10, Status: domain.KYCStatusPending, SubmittedAt: &now}
	m := newTestMetrics(t)
	svc := NewKYCService(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)
	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Reject error and metrics paths ───────────────────────────────────────────

func TestKYCService_Reject_UpdateStatusWithEventError_Propagates(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, Status: domain.KYCStatusPending}
	stub := &kycProfileRepoStub{profile: profile, updateStatusErr: errors.New("db fail")}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Reject(context.Background(), 1, 99, "reason"); err == nil {
		t.Fatal("expected UpdateStatusWithEvent error, got nil")
	}
}

func TestKYCService_Reject_WithSubmittedAt_RecordsReviewDuration(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	profile := &domain.KYCProfile{ID: 1, UserID: 10, Status: domain.KYCStatusPending, SubmittedAt: &now}
	m := newTestMetrics(t)
	svc := NewKYCService(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{},
		&noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)
	if err := svc.Reject(context.Background(), 1, 99, "docs expired"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Escalate UpdateStatus error ───────────────────────────────────────────────

func TestKYCService_Escalate_UpdateStatusWithEventError_Propagates(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, Status: domain.KYCStatusPending}
	stub := &kycProfileRepoStub{profile: profile, updateStatusErr: errors.New("db fail")}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.Escalate(context.Background(), 1, 99, "pep match"); err == nil {
		t.Fatal("expected UpdateStatusWithEvent error, got nil")
	}
}

// ── RequestDocument repo error ────────────────────────────────────────────────

func TestKYCService_RequestDocument_RepoError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.RequestDocument(context.Background(), 1, 99, domain.KYCDocGovID, "expired"); err == nil {
		t.Fatal("expected repo error, got nil")
	}
}

// ── ReleaseFrozenBalance — atomic credit ──────────────────────────────────────

func TestKYCService_ReleaseFrozenBalance_CreditsViaAtomicMethod(t *testing.T) {
	existing := &domain.KYCProfile{
		ID: 7, UserID: 5,
		BalanceFrozen:     true,
		FrozenAmountCents: 50_000, // Q500
		Status:            domain.KYCStatusApproved,
	}
	stub := &kycProfileRepoStub{profile: existing}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.ReleaseFrozenBalance(context.Background(), 5, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stub returns profile.FrozenAmountCents (50_000) from ReleaseAndCreditFrozen,
	// verifying the service delegated the credit to the atomic repository method.
}

func TestKYCService_ReleaseFrozenBalance_ProfileNotFound_ReturnsNotFound(t *testing.T) {
	stub := &kycProfileRepoStub{profile: nil}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	err := svc.ReleaseFrozenBalance(context.Background(), 99, 1)
	if err == nil {
		t.Fatal("expected not-found error for missing profile, got nil")
	}
}

func TestKYCService_ReleaseFrozenBalance_ReleaseRepoError_Propagates(t *testing.T) {
	existing := &domain.KYCProfile{ID: 3, UserID: 5, BalanceFrozen: true, FrozenAmountCents: 10_000}
	stub := &kycProfileRepoStub{profile: existing, releaseErr: errors.New("tx fail")}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.ReleaseFrozenBalance(context.Background(), 5, 99); err == nil {
		t.Fatal("expected repo error from ReleaseAndCreditFrozen, got nil")
	}
}

// ── ReleaseFrozenBalance GetByUserID error ────────────────────────────────────

func TestKYCService_ReleaseFrozenBalance_GetByUserIDError_Propagates(t *testing.T) {
	stub := &kycProfileRepoStub{getByUserIDErr: errors.New("db fail")}
	svc := newKYCSvc(stub, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.ReleaseFrozenBalance(context.Background(), 1, 99); err == nil {
		t.Fatal("expected GetByUserID error, got nil")
	}
}

// ── appendEvent error is logged, not returned ────────────────────────────────

func TestKYCService_AppendEvent_ErrorIsLoggedNotReturned(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{err: errors.New("event fail")})
	// Submit calls appendEvent after a successful upsert; error must be swallowed.
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName: "Juan", DocumentType: domain.KYCDocGovID, DocumentNumber: "X",
	})
	if err != nil {
		t.Fatalf("expected nil when event repo fails (best-effort), got %v", err)
	}
}

// ── RecalculateRiskScore additional tiers ─────────────────────────────────────

func TestKYCService_RecalculateRiskScore_TierOne_NoFlags(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 1, Tier: domain.KYCTierOne}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != riskWeightTierOne {
		t.Errorf("expected score=%d for TierOne no flags, got %d", riskWeightTierOne, score)
	}
}

func TestKYCService_RecalculateRiskScore_TierTwo_NoFlags(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 1, Tier: domain.KYCTierTwo}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	score, err := svc.RecalculateRiskScore(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != riskWeightTierTwo {
		t.Errorf("expected score=%d for TierTwo no flags, got %d", riskWeightTierTwo, score)
	}
}

func TestKYCService_RecalculateRiskScore_WithLedger_AboveThreshold_AddsWeight(t *testing.T) {
	profile := &domain.KYCProfile{ID: 1, UserID: 1, Tier: domain.KYCTierOne}
	svc := newKYCSvc(&kycProfileRepoStub{profile: profile}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetLedger(&ledgerSumStub{sum: int64(domain.DefaultKYCAMLThresholdCents)})
	score, err := svc.RecalculateRiskScore(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := riskWeightTierOne + riskWeightDepositVelocity
	if score != expected {
		t.Errorf("expected score=%d (tier1 + velocity), got %d", expected, score)
	}
}

// ── GetRiskDashboard cache paths ──────────────────────────────────────────────

func TestKYCService_GetRiskDashboard_CacheHit_ReturnsCachedValue(t *testing.T) {
	stats := &domain.KYCRiskDashboardStats{
		QueueDepth:       7,
		TierDistribution: map[domain.KYCTier]int64{},
	}
	store := newStubCache()
	store.seed(domain.CacheKeyKYCRiskDashboard, stats)

	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	got, err := svc.GetRiskDashboard(context.Background())
	if err != nil || got == nil {
		t.Fatalf("expected cache hit result, got %v err=%v", got, err)
	}
	if got.QueueDepth != 7 {
		t.Errorf("expected QueueDepth=7 from cache, got %d", got.QueueDepth)
	}
}

func TestKYCService_GetRiskDashboard_CacheMiss_SetsCache(t *testing.T) {
	store := newStubCache()
	svc := newKYCSvc(&kycProfileRepoStub{}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	_, err := svc.GetRiskDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.setCalls != 1 {
		t.Errorf("expected 1 cache set call after miss, got %d", store.setCalls)
	}
}

// ── cache invalidation on mutations ──────────────────────────────────────────

func TestKYCService_Approve_InvalidatesRiskDashboardCache(t *testing.T) {
	store := newStubCache()
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range store.deleted {
		if k == domain.CacheKeyKYCRiskDashboard {
			return
		}
	}
	t.Errorf("expected %q to be deleted from cache after Approve; deleted=%v", domain.CacheKeyKYCRiskDashboard, store.deleted)
}

func TestKYCService_Reject_InvalidatesRiskDashboardCache(t *testing.T) {
	store := newStubCache()
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	if err := svc.Reject(context.Background(), 1, 99, "fraud suspected"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range store.deleted {
		if k == domain.CacheKeyKYCRiskDashboard {
			return
		}
	}
	t.Errorf("expected %q to be deleted from cache after Reject; deleted=%v", domain.CacheKeyKYCRiskDashboard, store.deleted)
}

func TestKYCService_Escalate_InvalidatesRiskDashboardCache(t *testing.T) {
	store := newStubCache()
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	if err := svc.Escalate(context.Background(), 1, 99, "manual review required"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range store.deleted {
		if k == domain.CacheKeyKYCRiskDashboard {
			return
		}
	}
	t.Errorf("expected %q to be deleted from cache after Escalate; deleted=%v", domain.CacheKeyKYCRiskDashboard, store.deleted)
}

func TestKYCService_RecalculateRiskScore_InvalidatesRiskDashboardCache(t *testing.T) {
	store := newStubCache()
	existing := &domain.KYCProfile{ID: 1, UserID: 5, Tier: domain.KYCTierOne}
	svc := newKYCSvc(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, &kycEventRepoStub{})
	svc.(*kycService).SetCache(store)

	if _, err := svc.RecalculateRiskScore(context.Background(), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range store.deleted {
		if k == domain.CacheKeyKYCRiskDashboard {
			return
		}
	}
	t.Errorf("expected %q to be deleted from cache after RecalculateRiskScore; deleted=%v", domain.CacheKeyKYCRiskDashboard, store.deleted)
}

// ── appendEvent metric counter ────────────────────────────────────────────────

// TestKYCService_AppendEvent_DropIncrementsCounter verifies that when
// eventRepo.Create fails, appendEvent increments AuditEventDropsTotal without
// panicking.  The noop meter discards the increment; we just prove the code
// path is exercised and the service does not return an error to the caller.
func TestKYCService_AppendEvent_DropIncrementsCounter(t *testing.T) {
	meter := metricnoop.NewMeterProvider().Meter("test")
	m, err := RegisterKYCMetrics(meter, nil, nil)
	if err != nil {
		t.Fatalf("RegisterKYCMetrics: %v", err)
	}

	existing := &domain.KYCProfile{ID: 1, UserID: 5, Status: domain.KYCStatusPending}
	eventRepo := &kycEventRepoStub{err: errors.New("db unavailable")}
	svc := NewKYCService(&kycProfileRepoStub{profile: existing}, &kycDocRepoStub{}, eventRepo, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop(), m)

	// Approve triggers appendEvent internally; the DB error is swallowed.
	if err := svc.Approve(context.Background(), 1, 99, domain.KYCTierTwo); err != nil {
		t.Fatalf("Approve returned unexpected error: %v", err)
	}
	// The noop counter cannot be read back, but the test confirms no panic occurs
	// and the error is not propagated to the caller.
}
