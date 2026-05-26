package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type kycProfileRepoStub struct {
	profile   *domain.KYCProfile
	profiles  []*domain.KYCProfile
	frozen    []*domain.FrozenBalanceSummary
	dupExists bool
	err       error
}

func (r *kycProfileRepoStub) Upsert(_ context.Context, p *domain.KYCProfile) error {
	if r.err != nil {
		return r.err
	}
	p.ID = 1
	return nil
}
func (r *kycProfileRepoStub) GetByUserID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return r.profile, r.err
}
func (r *kycProfileRepoStub) GetByID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return r.profile, r.err
}
func (r *kycProfileRepoStub) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
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
func (r *kycProfileRepoStub) CountAccountsByDeviceFingerprint(_ context.Context, _ string, _ int) (int64, error) {
	return 0, r.err
}

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

func TestKYCService_FreezeBalance_SetFrozenError_Propagates(t *testing.T) {
	svc := newKYCSvc(&kycProfileRepoStub{err: errors.New("db fail")}, &kycDocRepoStub{}, &kycEventRepoStub{})
	if err := svc.FreezeBalance(context.Background(), 1, 1000, "reason"); err == nil {
		t.Fatal("expected error from SetFrozen, got nil")
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

// ── checkDeviceFraud ──────────────────────────────────────────────────────────

func TestKYCService_Submit_WithDeviceFingerprint_DuplicateDevice_ReturnsConflict(t *testing.T) {
	pr := &kycProfileRepoStub{}
	pr.dupExists = false
	// Use a custom stub that returns count > 0 for fingerprint check.
	// We can't easily test checkDeviceFraud in isolation, so drive it through Submit
	// by overriding the fingerprint counter inside the repo stub.
	// This exercises the branch where devCount > 0.
	dupPR := &fingerprintCollisionStub{kycProfileRepoStub: kycProfileRepoStub{}}
	svc := NewKYCService(dupPR, &kycDocRepoStub{}, &kycEventRepoStub{}, &noopSystemParamService{}, &noopAuditLogger{}, zap.NewNop())
	_, err := svc.Submit(context.Background(), 1, SubmitKYCRequest{
		FullName:          "Juan",
		DocumentType:      domain.KYCDocGovID,
		DocumentNumber:    "X",
		DeviceFingerprint: "abc123",
	})
	if err == nil {
		t.Fatal("expected conflict for device fingerprint collision, got nil")
	}
}

type fingerprintCollisionStub struct {
	kycProfileRepoStub
}

func (r *fingerprintCollisionStub) CountAccountsByDeviceFingerprint(_ context.Context, _ string, _ int) (int64, error) {
	return 1, nil
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
