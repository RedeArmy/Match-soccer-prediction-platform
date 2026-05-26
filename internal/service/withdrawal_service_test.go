package service

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type withdrawalReqRepoStub struct {
	req  *domain.WithdrawalRequest
	reqs []*domain.WithdrawalRequest
	err  error
}

func (r *withdrawalReqRepoStub) CreateAndReserve(_ context.Context, req *domain.WithdrawalRequest) error {
	if r.err != nil {
		return r.err
	}
	req.ID = 55
	req.Status = domain.WithdrawalPending
	return nil
}
func (r *withdrawalReqRepoStub) GetByID(_ context.Context, _ int) (*domain.WithdrawalRequest, error) {
	return r.req, r.err
}
func (r *withdrawalReqRepoStub) ListByUser(_ context.Context, _ int) ([]*domain.WithdrawalRequest, error) {
	return r.reqs, r.err
}
func (r *withdrawalReqRepoStub) ListPending(_ context.Context) ([]*domain.WithdrawalRequest, error) {
	return r.reqs, r.err
}
func (r *withdrawalReqRepoStub) Approve(_ context.Context, _ int, _ int, _ string) (*domain.WithdrawalRequest, error) {
	return r.req, r.err
}
func (r *withdrawalReqRepoStub) RejectAndRelease(_ context.Context, _ int, _ int, _ string) (*domain.WithdrawalRequest, error) {
	return r.req, r.err
}
func (r *withdrawalReqRepoStub) MarkProcessedAndCommit(_ context.Context, _ int) (*domain.WithdrawalRequest, error) {
	return r.req, r.err
}

// withdrawalParamRepo returns configurable values per key for limit tests.
type withdrawalParamRepo struct {
	params map[string]string
	err    error
}

func (r *withdrawalParamRepo) Get(_ context.Context, key string) (*domain.SystemParam, error) {
	if r.err != nil {
		return nil, r.err
	}
	if v, ok := r.params[key]; ok {
		return &domain.SystemParam{Key: key, Value: v}, nil
	}
	return nil, nil
}
func (r *withdrawalParamRepo) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (r *withdrawalParamRepo) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (r *withdrawalParamRepo) Set(_ context.Context, k, v string, _ int) (*domain.SystemParam, error) {
	return &domain.SystemParam{Key: k, Value: v}, nil
}
func (r *withdrawalParamRepo) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (r *withdrawalParamRepo) ResetToDefault(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}

func newWithdrawalSvc(wr *withdrawalReqRepoStub, pr *withdrawalParamRepo) WithdrawalService {
	if pr == nil {
		pr = &withdrawalParamRepo{}
	}
	return NewWithdrawalService(wr, pr, NoopKYCGate{}, nil, &noopAuditLogger{}, zap.NewNop())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestWithdrawalService_Create_HappyPath_ReturnsRequest(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, nil)

	got, err := svc.Create(context.Background(), 1, domain.DefaultWithdrawalMinCents, "GTQ", domain.WithdrawalMethodBankGT, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 55 {
		t.Errorf("expected request with ID 55, got %v", got)
	}
	if got.Status != domain.WithdrawalPending {
		t.Errorf("status: got %q, want pending", got.Status)
	}
}

func TestWithdrawalService_Create_DefaultsCurrencyToGTQ(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, nil)

	got, err := svc.Create(context.Background(), 1, domain.DefaultWithdrawalMinCents, "", domain.WithdrawalMethodBankGT, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Currency != "GTQ" {
		t.Errorf("currency: got %q, want GTQ", got.Currency)
	}
}

func TestWithdrawalService_Create_AmountBelowDefaultMinimum_ReturnsValidation(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, nil)

	_, err := svc.Create(context.Background(), 1, domain.DefaultWithdrawalMinCents-1, "GTQ", domain.WithdrawalMethodBankGT, nil)
	if err == nil {
		t.Fatal("expected validation error for amount below minimum, got nil")
	}
}

func TestWithdrawalService_Create_AmountAboveDefaultMaximum_ReturnsValidation(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, nil)

	_, err := svc.Create(context.Background(), 1, domain.DefaultWithdrawalMaxCents+1, "GTQ", domain.WithdrawalMethodBankGT, nil)
	if err == nil {
		t.Fatal("expected validation error for amount above maximum, got nil")
	}
}

func TestWithdrawalService_Create_CustomMinFromParams_Respected(t *testing.T) {
	customMin := 1000
	pr := &withdrawalParamRepo{params: map[string]string{
		domain.ParamKeyWithdrawalMinCents: strconv.Itoa(customMin),
	}}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, pr)

	// amount between custom min and default max — should succeed
	_, err := svc.Create(context.Background(), 1, customMin, "GTQ", domain.WithdrawalMethodBankGT, nil)
	if err != nil {
		t.Fatalf("unexpected error with custom min: %v", err)
	}
}

func TestWithdrawalService_Create_CustomMaxFromParams_Respected(t *testing.T) {
	customMax := 100_000
	pr := &withdrawalParamRepo{params: map[string]string{
		domain.ParamKeyWithdrawalMaxCents: strconv.Itoa(customMax),
	}}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{}, pr)

	_, err := svc.Create(context.Background(), 1, customMax+1, "GTQ", domain.WithdrawalMethodBankGT, nil)
	if err == nil {
		t.Fatal("expected validation error above custom max, got nil")
	}
}

func TestWithdrawalService_Create_RepoErrorPropagates(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{err: errors.New("insufficient balance")}, nil)

	_, err := svc.Create(context.Background(), 1, domain.DefaultWithdrawalMinCents, "GTQ", domain.WithdrawalMethodPayPal, nil)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestWithdrawalService_GetByID_ReturnsRequest(t *testing.T) {
	req := &domain.WithdrawalRequest{ID: 7, AmountCents: 5000}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{req: req}, nil)

	got, err := svc.GetByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != 7 {
		t.Errorf("expected request ID 7, got %v", got)
	}
}

func TestWithdrawalService_GetByID_NotFound(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{req: nil}, nil)

	got, err := svc.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for not found, got %v", got)
	}
}

// ── ListByUser ────────────────────────────────────────────────────────────────

func TestWithdrawalService_ListByUser_ReturnsAll(t *testing.T) {
	reqs := []*domain.WithdrawalRequest{{ID: 1}, {ID: 2}}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{reqs: reqs}, nil)

	got, err := svc.ListByUser(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 requests, got %d", len(got))
	}
}

// ── ListPending ───────────────────────────────────────────────────────────────

func TestWithdrawalService_ListPending_ReturnsAll(t *testing.T) {
	reqs := []*domain.WithdrawalRequest{{ID: 3, Status: domain.WithdrawalPending}}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{reqs: reqs}, nil)

	got, err := svc.ListPending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 pending request, got %d", len(got))
	}
}

// ── ApproveRequest ────────────────────────────────────────────────────────────

func TestWithdrawalService_ApproveRequest_HappyPath(t *testing.T) {
	approved := &domain.WithdrawalRequest{ID: 1, Status: domain.WithdrawalApproved}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{req: approved}, nil)

	got, err := svc.ApproveRequest(context.Background(), 1, 99, "looks good")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.WithdrawalApproved {
		t.Errorf("status: got %q, want approved", got.Status)
	}
}

func TestWithdrawalService_ApproveRequest_RepoErrorPropagates(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{err: errors.New("not found")}, nil)

	_, err := svc.ApproveRequest(context.Background(), 999, 99, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── RejectRequest ─────────────────────────────────────────────────────────────

func TestWithdrawalService_RejectRequest_HappyPath(t *testing.T) {
	rejected := &domain.WithdrawalRequest{ID: 2, Status: domain.WithdrawalRejected}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{req: rejected}, nil)

	got, err := svc.RejectRequest(context.Background(), 2, 99, "invalid account")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.WithdrawalRejected {
		t.Errorf("status: got %q, want rejected", got.Status)
	}
}

func TestWithdrawalService_RejectRequest_RepoErrorPropagates(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{err: errors.New("not found")}, nil)

	_, err := svc.RejectRequest(context.Background(), 999, 99, "reason")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ProcessWithdrawal ─────────────────────────────────────────────────────────

func TestWithdrawalService_ProcessWithdrawal_HappyPath(t *testing.T) {
	processed := &domain.WithdrawalRequest{ID: 3, Status: domain.WithdrawalProcessed}
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{req: processed}, nil)

	got, err := svc.ProcessWithdrawal(context.Background(), 3, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.WithdrawalProcessed {
		t.Errorf("status: got %q, want processed", got.Status)
	}
}

func TestWithdrawalService_ProcessWithdrawal_RepoErrorPropagates(t *testing.T) {
	svc := newWithdrawalSvc(&withdrawalReqRepoStub{err: errors.New("not approved")}, nil)

	_, err := svc.ProcessWithdrawal(context.Background(), 999, 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
