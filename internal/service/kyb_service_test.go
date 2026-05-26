package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type kybRepoStub struct {
	profile   *domain.KYBProfile
	profiles  []*domain.KYBProfile
	dupExists bool
	err       error
}

func (r *kybRepoStub) Create(_ context.Context, p *domain.KYBProfile) error {
	if r.err != nil {
		return r.err
	}
	p.ID = 1
	return nil
}
func (r *kybRepoStub) GetByUserID(_ context.Context, _ int) (*domain.KYBProfile, error) {
	return r.profile, r.err
}
func (r *kybRepoStub) GetByID(_ context.Context, _ int) (*domain.KYBProfile, error) {
	return r.profile, r.err
}
func (r *kybRepoStub) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
	return r.err
}
func (r *kybRepoStub) ListPending(_ context.Context, _, _ int) ([]*domain.KYBProfile, error) {
	return r.profiles, r.err
}
func (r *kybRepoStub) CountByStatus(_ context.Context) (map[domain.KYCStatus]int64, error) {
	return nil, r.err
}
func (r *kybRepoStub) ExistsByTaxIDAndJurisdiction(_ context.Context, _, _ string, _ int) (bool, error) {
	return r.dupExists, r.err
}

func newKYBSvc(repo *kybRepoStub) KYBService {
	return NewKYBService(repo, &kycEventRepoStub{}, &noopAuditLogger{}, nil, zap.NewNop())
}

func validKYBInput() KYBSubmitInput {
	d := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	return KYBSubmitInput{
		LegalName:         "Acme S.A.",
		TaxID:             "CF12345",
		Jurisdiction:      "GT",
		UBOName:           "Juan Pérez",
		IncorporationDate: &d,
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestKYBService_Submit_Success(t *testing.T) {
	repo := &kybRepoStub{}
	svc := newKYBSvc(repo)
	p, err := svc.Submit(context.Background(), 1, validKYBInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil || p.ID != 1 {
		t.Errorf("expected profile ID 1, got %+v", p)
	}
	if p.Status != domain.KYCStatusPending {
		t.Errorf("expected pending status, got %s", p.Status)
	}
}

func TestKYBService_Submit_MissingLegalName_Returns422(t *testing.T) {
	repo := &kybRepoStub{}
	svc := newKYBSvc(repo)
	input := validKYBInput()
	input.LegalName = ""
	_, err := svc.Submit(context.Background(), 1, input)
	if !isValidation(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestKYBService_Submit_DuplicateTaxID_Returns409(t *testing.T) {
	repo := &kybRepoStub{dupExists: true}
	svc := newKYBSvc(repo)
	_, err := svc.Submit(context.Background(), 1, validKYBInput())
	if !isConflict(err) {
		t.Errorf("expected conflict error for duplicate tax_id, got %v", err)
	}
}

func TestKYBService_Approve_TransitionsStatus(t *testing.T) {
	status := domain.KYCStatusPending
	repo := &kybRepoStub{profile: &domain.KYBProfile{ID: 1, UserID: 5, Status: status}}
	svc := newKYBSvc(repo)
	if err := svc.Approve(context.Background(), 1, 99); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKYBService_Reject_RequiresReason(t *testing.T) {
	status := domain.KYCStatusPending
	repo := &kybRepoStub{profile: &domain.KYBProfile{ID: 1, UserID: 5, Status: status}}
	svc := newKYBSvc(repo)
	if err := svc.Reject(context.Background(), 1, 99, ""); !isValidation(err) {
		t.Errorf("expected validation error for missing reason, got %v", err)
	}
}

func TestKYBService_Reject_Success(t *testing.T) {
	status := domain.KYCStatusPending
	repo := &kybRepoStub{profile: &domain.KYBProfile{ID: 1, UserID: 5, Status: status}}
	svc := newKYBSvc(repo)
	if err := svc.Reject(context.Background(), 1, 99, "documents expired"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func isValidation(err error) bool {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return ae.Code == apperrors.CodeValidation
	}
	return false
}

func isConflict(err error) bool {
	var ae *apperrors.AppError
	return errors.As(err, &ae) && ae.Code == apperrors.CodeConflict
}

// ── GetStatus / ListPending / GetByID ─────────────────────────────────────────

func TestKYBService_GetStatus_ReturnsProfile(t *testing.T) {
	p := &domain.KYBProfile{ID: 1, UserID: 3, Status: domain.KYCStatusPending}
	svc := newKYBSvc(&kybRepoStub{profile: p})
	got, err := svc.GetStatus(context.Background(), 3)
	if err != nil || got == nil || got.ID != 1 {
		t.Errorf("expected profile id=1, got %v err=%v", got, err)
	}
}

func TestKYBService_GetStatus_Nil_WhenNotFound(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{profile: nil})
	got, err := svc.GetStatus(context.Background(), 99)
	if err != nil || got != nil {
		t.Errorf("expected nil profile, got %v err=%v", got, err)
	}
}

func TestKYBService_ListPending_ReturnsList(t *testing.T) {
	profiles := []*domain.KYBProfile{{ID: 1}, {ID: 2}}
	svc := newKYBSvc(&kybRepoStub{profiles: profiles})
	got, err := svc.ListPending(context.Background(), 10, 0)
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 profiles, got %d err=%v", len(got), err)
	}
}

func TestKYBService_GetByID_ReturnsProfile(t *testing.T) {
	p := &domain.KYBProfile{ID: 7, UserID: 3}
	svc := newKYBSvc(&kybRepoStub{profile: p})
	got, err := svc.GetByID(context.Background(), 7)
	if err != nil || got == nil || got.ID != 7 {
		t.Errorf("expected profile id=7, got %v err=%v", got, err)
	}
}

// ── Approve additional edge cases ─────────────────────────────────────────────

func TestKYBService_Approve_AlreadyApproved_ReturnsConflict(t *testing.T) {
	repo := &kybRepoStub{profile: &domain.KYBProfile{ID: 1, Status: domain.KYCStatusApproved}}
	svc := newKYBSvc(repo)
	if err := svc.Approve(context.Background(), 1, 99); !isConflict(err) {
		t.Errorf("expected conflict for already-approved profile, got %v", err)
	}
}

func TestKYBService_Approve_ProfileNotFound_ReturnsNotFound(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{profile: nil})
	err := svc.Approve(context.Background(), 999, 99)
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestKYBService_Approve_RepoError_Propagates(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{err: errors.New("db fail")})
	if err := svc.Approve(context.Background(), 1, 99); err == nil {
		t.Fatal("expected repo error, got nil")
	}
}

// ── Reject additional edge cases ──────────────────────────────────────────────

func TestKYBService_Reject_AlreadyRejected_ReturnsConflict(t *testing.T) {
	repo := &kybRepoStub{profile: &domain.KYBProfile{ID: 1, Status: domain.KYCStatusRejected}}
	svc := newKYBSvc(repo)
	if err := svc.Reject(context.Background(), 1, 99, "docs expired"); !isConflict(err) {
		t.Errorf("expected conflict for already-rejected profile, got %v", err)
	}
}

func TestKYBService_Reject_ProfileNotFound_ReturnsNotFound(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{profile: nil})
	if err := svc.Reject(context.Background(), 999, 99, "reason"); err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestKYBService_Submit_AlreadyPending_ReturnsConflict(t *testing.T) {
	existing := &domain.KYBProfile{Status: domain.KYCStatusPending}
	svc := newKYBSvc(&kybRepoStub{profile: existing})
	_, err := svc.Submit(context.Background(), 1, validKYBInput())
	if !isConflict(err) {
		t.Errorf("expected conflict for pending profile, got %v", err)
	}
}

func TestKYBService_Submit_RepoError_Propagates(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{err: errors.New("db fail")})
	if _, err := svc.Submit(context.Background(), 1, validKYBInput()); err == nil {
		t.Fatal("expected repo error, got nil")
	}
}

func TestKYBService_Submit_MissingTaxID_Returns422(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{})
	input := validKYBInput()
	input.TaxID = ""
	if _, err := svc.Submit(context.Background(), 1, input); !isValidation(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestKYBService_Submit_MissingJurisdiction_Returns422(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{})
	input := validKYBInput()
	input.Jurisdiction = ""
	if _, err := svc.Submit(context.Background(), 1, input); !isValidation(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestKYBService_Submit_MissingUBOName_Returns422(t *testing.T) {
	svc := newKYBSvc(&kybRepoStub{})
	input := validKYBInput()
	input.UBOName = ""
	if _, err := svc.Submit(context.Background(), 1, input); !isValidation(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}
