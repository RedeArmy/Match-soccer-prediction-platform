package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── router ────────────────────────────────────────────────────────────────────

func newAdminKYCRouter(svc *stubKYCSvc) http.Handler {
	h := handler.NewAdminKYCHandler(svc, zap.NewNop())
	r := chi.NewRouter()
	r.Get("/kyc/queue", h.ListQueue)
	r.Get("/kyc/profiles/{profileID}", h.GetProfile)
	r.Post("/kyc/profiles/{profileID}/approve", h.Approve)
	r.Post("/kyc/profiles/{profileID}/reject", h.Reject)
	r.Post("/kyc/profiles/{profileID}/escalate", h.Escalate)
	r.Post("/kyc/profiles/{profileID}/request-doc", h.RequestDocument)
	r.Post("/kyc/documents/{docID}/verify", h.VerifyDocument)
	r.Get("/kyc/profiles/{profileID}/documents", h.ListDocumentsForProfile)
	r.Get("/kyc/frozen-balances", h.ListFrozenBalances)
	r.Post("/kyc/users/{userID}/release-freeze", h.ReleaseFrozenBalance)
	r.Get("/kyc/profiles/{profileID}/events", h.ListProfileEvents)
	r.Get("/kyc/risk-dashboard", h.RiskDashboard)
	return r
}

var kycProfileFixture = &domain.KYCProfile{
	ID:        1,
	UserID:    10,
	Status:    domain.KYCStatusPending,
	CreatedAt: time.Now(),
	UpdatedAt: time.Now(),
}

// ── ListQueue ─────────────────────────────────────────────────────────────────

func TestAdminKYCHandler_ListQueue_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/queue", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ListQueue_WithStatusFilter_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/queue?status=pending", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ListQueue_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/queue", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── GetProfile ────────────────────────────────────────────────────────────────

func TestAdminKYCHandler_GetProfile_Found_Returns200(t *testing.T) {
	svc := &stubKYCSvc{profile: kycProfileFixture}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_GetProfile_NotFound_Returns404(t *testing.T) {
	svc := &stubKYCSvc{profile: nil}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminKYCHandler_GetProfile_InvalidID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/not-a-number", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminKYCHandler_GetProfile_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── Approve ───────────────────────────────────────────────────────────────────

func TestAdminKYCHandler_Approve_ValidTier_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/approve", `{"tier":2}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_Approve_InvalidTier_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/approve", `{"tier":99}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminKYCHandler_Approve_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/profiles/1/approve", `{"tier":2}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_Approve_InvalidProfileID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/bad/approve", `{"tier":2}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminKYCHandler_Approve_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/approve", `{"tier":2}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── Reject ────────────────────────────────────────────────────────────────────

func TestAdminKYCHandler_Reject_WithReason_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/reject", `{"reason":"documents expired"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_Reject_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/profiles/1/reject", `{"reason":"expired"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_Reject_InvalidProfileID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/bad/reject", `{"reason":"x"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── Escalate ──────────────────────────────────────────────────────────────────

func TestAdminKYCHandler_Escalate_WithReason_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/escalate", `{"reason":"sanctions match"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_Escalate_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/profiles/1/escalate", `{"reason":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_Escalate_InvalidProfileID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/bad/escalate", `{"reason":"x"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── RequestDocument ───────────────────────────────────────────────────────────

func TestAdminKYCHandler_RequestDocument_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/1/request-doc",
		`{"document_type":"gov_id","reason":"expired"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_RequestDocument_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/profiles/1/request-doc", `{"document_type":"gov_id"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_RequestDocument_InvalidProfileID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/profiles/bad/request-doc",
		`{"document_type":"gov_id"}`), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── VerifyDocument ────────────────────────────────────────────────────────────

func TestAdminKYCHandler_VerifyDocument_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/documents/42/verify", ""), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_VerifyDocument_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/documents/42/verify", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_VerifyDocument_InvalidDocID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/documents/bad/verify", ""), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── ListDocumentsForProfile ───────────────────────────────────────────────────

func TestAdminKYCHandler_ListDocumentsForProfile_Returns200(t *testing.T) {
	svc := &stubKYCSvc{profile: kycProfileFixture}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1/documents", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ListDocumentsForProfile_ProfileNotFound_Returns404(t *testing.T) {
	svc := &stubKYCSvc{profile: nil}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1/documents", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminKYCHandler_ListDocumentsForProfile_InvalidID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/bad/documents", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── ListFrozenBalances ────────────────────────────────────────────────────────

func TestAdminKYCHandler_ListFrozenBalances_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/frozen-balances", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ListFrozenBalances_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/frozen-balances", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── ReleaseFrozenBalance ──────────────────────────────────────────────────────

func TestAdminKYCHandler_ReleaseFrozenBalance_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/users/10/release-freeze", ""), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ReleaseFrozenBalance_Unauthenticated_Returns401(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodPost, "/kyc/users/10/release-freeze", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminKYCHandler_ReleaseFrozenBalance_InvalidUserID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/kyc/users/bad/release-freeze", ""), adminCaller)
	w := doReq(newAdminKYCRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── ListProfileEvents ─────────────────────────────────────────────────────────

func TestAdminKYCHandler_ListProfileEvents_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/1/events", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_ListProfileEvents_InvalidProfileID_Returns422(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/profiles/bad/events", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── RiskDashboard ─────────────────────────────────────────────────────────────

func TestAdminKYCHandler_RiskDashboard_Returns200(t *testing.T) {
	svc := &stubKYCSvc{}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/risk-dashboard", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminKYCHandler_RiskDashboard_SvcError_Returns500(t *testing.T) {
	svc := &stubKYCSvc{err: apperrors.Internal(nil)}
	w := do(newAdminKYCRouter(svc), http.MethodGet, "/kyc/risk-dashboard", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
