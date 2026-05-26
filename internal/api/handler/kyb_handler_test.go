package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stub ──────────────────────────────────────────────────────────────────────

type stubKYBSvc struct {
	profile  *domain.KYBProfile
	profiles []*domain.KYBProfile
	err      error
}

func (s *stubKYBSvc) Submit(_ context.Context, _ int, _ service.KYBSubmitInput) (*domain.KYBProfile, error) {
	return s.profile, s.err
}
func (s *stubKYBSvc) GetStatus(_ context.Context, _ int) (*domain.KYBProfile, error) {
	return s.profile, s.err
}
func (s *stubKYBSvc) ListPending(_ context.Context, _, _ int) ([]*domain.KYBProfile, error) {
	return s.profiles, s.err
}
func (s *stubKYBSvc) GetByID(_ context.Context, _ int) (*domain.KYBProfile, error) {
	return s.profile, s.err
}
func (s *stubKYBSvc) Approve(_ context.Context, _, _ int) error          { return s.err }
func (s *stubKYBSvc) Reject(_ context.Context, _, _ int, _ string) error { return s.err }

// ── routers ───────────────────────────────────────────────────────────────────

func newKYBRouter(svc *stubKYBSvc) http.Handler {
	h := handler.NewKYBHandler(svc, zap.NewNop())
	r := chi.NewRouter()
	r.Get("/kyb/status", h.GetStatus)
	r.Post("/kyb/submit", h.Submit)
	return r
}

func newAdminKYBRouter(svc *stubKYBSvc) http.Handler {
	h := handler.NewAdminKYBHandler(svc, zap.NewNop())
	r := chi.NewRouter()
	r.Get("/admin/kyb/queue", h.ListQueue)
	r.Get("/admin/kyb/profiles/{id}", h.GetProfile)
	r.Post("/admin/kyb/profiles/{id}/approve", h.Approve)
	r.Post("/admin/kyb/profiles/{id}/reject", h.Reject)
	return r
}

var kybProfileFixture = &domain.KYBProfile{
	ID:        1,
	UserID:    10,
	Status:    domain.KYCStatusPending,
	CreatedAt: time.Now(),
	UpdatedAt: time.Now(),
}

// ── KYBHandler.GetStatus ──────────────────────────────────────────────────────

func TestKYBHandler_GetStatus_NoProfile_ReturnsUnverified(t *testing.T) {
	svc := &stubKYBSvc{profile: nil}
	req := withCaller(newAdminRequest(http.MethodGet, "/kyb/status", ""), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestKYBHandler_GetStatus_WithProfile_Returns200(t *testing.T) {
	svc := &stubKYBSvc{profile: kybProfileFixture}
	req := withCaller(newAdminRequest(http.MethodGet, "/kyb/status", ""), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestKYBHandler_GetStatus_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYBSvc{}
	w := do(newKYBRouter(svc), http.MethodGet, "/kyb/status", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestKYBHandler_GetStatus_SvcError_Returns500(t *testing.T) {
	svc := &stubKYBSvc{err: apperrors.Internal(nil)}
	req := withCaller(newAdminRequest(http.MethodGet, "/kyb/status", ""), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── KYBHandler.Submit ─────────────────────────────────────────────────────────

func TestKYBHandler_Submit_ValidBody_Returns201(t *testing.T) {
	svc := &stubKYBSvc{profile: kybProfileFixture}
	body := `{"legal_name":"Acme S.A.","tax_id":"CF1","jurisdiction":"GT","ubo_name":"Juan"}`
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyb/submit", body), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestKYBHandler_Submit_BadIncorporationDate_Returns422(t *testing.T) {
	svc := &stubKYBSvc{}
	body := `{"legal_name":"Acme","tax_id":"CF1","jurisdiction":"GT","ubo_name":"Juan","incorporation_date":"not-a-date"}`
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyb/submit", body), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestKYBHandler_Submit_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYBSvc{}
	body := `{"legal_name":"Acme","tax_id":"CF1","jurisdiction":"GT","ubo_name":"Juan"}`
	w := do(newKYBRouter(svc), http.MethodPost, "/kyb/submit", body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestKYBHandler_Submit_ValidIncorporationDate_Returns201(t *testing.T) {
	svc := &stubKYBSvc{profile: kybProfileFixture}
	body := `{"legal_name":"Acme","tax_id":"CF1","jurisdiction":"GT","ubo_name":"Juan","incorporation_date":"2010-06-15"}`
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyb/submit", body), adminCaller)
	w := doReq(newKYBRouter(svc), req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// ── AdminKYBHandler.ListQueue ─────────────────────────────────────────────────

func TestAdminKYBHandler_ListQueue_Returns200(t *testing.T) {
	svc := &stubKYBSvc{}
	w := do(newAdminKYBRouter(svc), http.MethodGet, "/admin/kyb/queue", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYBHandler_ListQueue_SvcError_Returns500(t *testing.T) {
	svc := &stubKYBSvc{err: apperrors.Internal(nil)}
	w := do(newAdminKYBRouter(svc), http.MethodGet, "/admin/kyb/queue", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminKYBHandler.GetProfile ────────────────────────────────────────────────

func TestAdminKYBHandler_GetProfile_Found_Returns200(t *testing.T) {
	svc := &stubKYBSvc{profile: kybProfileFixture}
	w := do(newAdminKYBRouter(svc), http.MethodGet, "/admin/kyb/profiles/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYBHandler_GetProfile_NotFound_Returns404(t *testing.T) {
	svc := &stubKYBSvc{profile: nil}
	w := do(newAdminKYBRouter(svc), http.MethodGet, "/admin/kyb/profiles/1", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminKYBHandler_GetProfile_InvalidID_Returns422(t *testing.T) {
	svc := &stubKYBSvc{}
	w := do(newAdminKYBRouter(svc), http.MethodGet, "/admin/kyb/profiles/bad", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminKYBHandler.Approve ───────────────────────────────────────────────────

func TestAdminKYBHandler_Approve_Returns200(t *testing.T) {
	svc := &stubKYBSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/admin/kyb/profiles/1/approve", ""), adminCaller)
	w := doReq(newAdminKYBRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYBHandler_Approve_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYBSvc{}
	w := do(newAdminKYBRouter(svc), http.MethodPost, "/admin/kyb/profiles/1/approve", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYBHandler_Approve_InvalidID_Returns422(t *testing.T) {
	svc := &stubKYBSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/admin/kyb/profiles/bad/approve", ""), adminCaller)
	w := doReq(newAdminKYBRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminKYBHandler.Reject ────────────────────────────────────────────────────

func TestAdminKYBHandler_Reject_WithReason_Returns200(t *testing.T) {
	svc := &stubKYBSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/admin/kyb/profiles/1/reject", `{"reason":"docs expired"}`), adminCaller)
	w := doReq(newAdminKYBRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYBHandler_Reject_EmptyReason_Returns422(t *testing.T) {
	svc := &stubKYBSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/admin/kyb/profiles/1/reject", `{"reason":""}`), adminCaller)
	w := doReq(newAdminKYBRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminKYBHandler_Reject_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYBSvc{}
	w := do(newAdminKYBRouter(svc), http.MethodPost, "/admin/kyb/profiles/1/reject", `{"reason":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYBHandler_Reject_InvalidID_Returns422(t *testing.T) {
	svc := &stubKYBSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/admin/kyb/profiles/bad/reject", `{"reason":"x"}`), adminCaller)
	w := doReq(newAdminKYBRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}
