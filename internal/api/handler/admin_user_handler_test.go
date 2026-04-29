package handler_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

const (
	adminUserPathBan     = "/users/5/ban"
	adminUserPathBulkBan = "/users/bulk-ban"
	adminUserPathUsers   = "/users"
)

func newAdminUserRouter(svc *stubAdminUserSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminUserHandler(svc, zap.NewNop())
	r.Get(adminUserPathUsers, h.ListUsers)
	r.Get("/users/{id}", h.GetUserProfile)
	r.Post("/users/{id}/ban", h.BanUser)
	r.Delete("/users/{id}/ban", h.UnbanUser)
	r.Post(adminUserPathBulkBan, h.BulkBan)
	return r
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

func TestAdminListUsers_Success_Returns200(t *testing.T) {
	svc := &stubAdminUserSvc{users: []*domain.User{{ID: 1, Name: "alice"}}}
	w := do(newAdminUserRouter(svc), http.MethodGet, adminUserPathUsers, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListUsers_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminUserSvc{err: errors.New("db error")}
	w := do(newAdminUserRouter(svc), http.MethodGet, adminUserPathUsers, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminListUsers_WithFilters_Returns200(t *testing.T) {
	svc := &stubAdminUserSvc{users: []*domain.User{}}
	req := newAdminRequest(http.MethodGet, "/users?banned=true&role=admin&search=bob", "")
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListUsers_LimitExceedsMax_CappedAndReturns200(t *testing.T) {
	svc := &stubAdminUserSvc{users: []*domain.User{}}
	req := newAdminRequest(http.MethodGet, "/users?limit=9999", "")
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

// ── GetUserProfile ────────────────────────────────────────────────────────────

func TestAdminGetUserProfile_Success_Returns200(t *testing.T) {
	profile := &service.AdminUserProfile{
		User:        &domain.User{ID: 1, Name: "alice"},
		Memberships: []*domain.GroupMembership{},
		Payments:    []*domain.PaymentRecord{},
	}
	svc := &stubAdminUserSvc{profile: profile}
	w := do(newAdminUserRouter(svc), http.MethodGet, "/users/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminGetUserProfile_InvalidID_Returns400(t *testing.T) {
	svc := &stubAdminUserSvc{}
	w := do(newAdminUserRouter(svc), http.MethodGet, "/users/abc", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminGetUserProfile_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminUserSvc{err: errors.New("not found")}
	w := do(newAdminUserRouter(svc), http.MethodGet, "/users/1", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── BanUser ───────────────────────────────────────────────────────────────────

func TestAdminBanUser_Success_Returns200(t *testing.T) {
	banned := &domain.User{ID: 5}
	svc := &stubAdminUserSvc{user: banned}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBan, `{"reason":"spam"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminBanUser_InvalidID_Returns400(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/users/abc/ban", `{"reason":"x"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBanUser_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminUserSvc{}
	w := do(newAdminUserRouter(svc), http.MethodPost, adminUserPathBan, `{"reason":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminBanUser_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBan, `not-json`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBanUser_EmptyReason_Returns422(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBan, `{"reason":""}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBanUser_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminUserSvc{err: errors.New("fail")}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBan, `{"reason":"abuse"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── UnbanUser ─────────────────────────────────────────────────────────────────

func TestAdminUnbanUser_Success_Returns200(t *testing.T) {
	svc := &stubAdminUserSvc{user: &domain.User{ID: 5}}
	req := withCaller(
		newAdminRequest(http.MethodDelete, adminUserPathBan, ""),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminUnbanUser_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminUserSvc{}
	w := do(newAdminUserRouter(svc), http.MethodDelete, adminUserPathBan, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminUnbanUser_InvalidID_Returns400(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequest(http.MethodDelete, "/users/abc/ban", ""),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── BulkBan ───────────────────────────────────────────────────────────────────

func TestAdminBulkBan_AllSuccess_Returns200(t *testing.T) {
	svc := &stubAdminUserSvc{
		bulkResult: service.BulkBanResult{Banned: []int{1, 2}, Failed: []service.BulkBanError{}},
	}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `{"user_ids":[1,2],"reason":"bots"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminBulkBan_PartialFailure_Returns207(t *testing.T) {
	svc := &stubAdminUserSvc{
		bulkResult: service.BulkBanResult{
			Banned: []int{1},
			Failed: []service.BulkBanError{{UserID: 2, Message: "not found"}},
		},
	}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `{"user_ids":[1,2],"reason":"bots"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusMultiStatus {
		t.Errorf("expected 207, got %d", w.Code)
	}
}

func TestAdminBulkBan_AllFailed_Returns207(t *testing.T) {
	svc := &stubAdminUserSvc{
		bulkResult: service.BulkBanResult{
			Banned: []int{},
			Failed: []service.BulkBanError{{UserID: 1, Message: "not found"}},
		},
	}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `{"user_ids":[1],"reason":"bots"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusMultiStatus {
		t.Errorf("expected 207, got %d", w.Code)
	}
}

func TestAdminBulkBan_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminUserSvc{}
	w := do(newAdminUserRouter(svc), http.MethodPost, adminUserPathBulkBan, `{"user_ids":[1],"reason":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminBulkBan_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `not-json`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkBan_EmptyBody_Returns422(t *testing.T) {
	svc := &stubAdminUserSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `{"user_ids":[],"reason":""}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkBan_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminUserSvc{err: errors.New("db failure")}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminUserPathBulkBan, `{"user_ids":[1],"reason":"bots"}`),
		adminCaller,
	)
	w := doReq(newAdminUserRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
