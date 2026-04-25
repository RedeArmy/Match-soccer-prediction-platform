package handler_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

const (
	adminOtherDBError             = "db error"
	adminOtherPathDLQ             = "/dlq"
	adminOtherPathDLQReplay       = "/dlq/replay"
	adminOtherPathSysParamsBulk   = "/system-params/bulk"
	adminOtherPathSysParams       = "/system-params"
	adminOtherPathLeaderboard     = "/leaderboard"
	adminOtherPathPaymentsPending = "/payments/pending"
	adminOtherPathGroups1         = "/groups/1"
	adminOtherPathGroups1Settings = "/groups/1/settings"
	adminOtherPathGroups1Transfer = "/groups/1/transfer-ownership"
	adminOtherPathTiebreakerSubs  = "/tiebreaker/submissions"
	adminOtherPathAuditLog        = "/audit-log"
	adminOtherPathConflicts       = "/conflicts"
	adminOtherPathConflictResolve = "/conflicts/group_without_owner/1/resolve"
	adminOtherScoringExact        = "scoring.exact"
)

// ── AdminGroupHandler ─────────────────────────────────────────────────────────

func newAdminGroupRouter(svc *stubAdminGroupSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminGroupHandler(svc, zap.NewNop())
	r.Delete("/groups/{id}", h.DeleteGroup)
	r.Delete("/groups/{id}/members/{membershipID}", h.RemoveMember)
	r.Patch("/groups/{id}/settings", h.UpdateGroupSettings)
	r.Post("/groups/{id}/transfer-ownership", h.TransferOwnership)
	return r
}

func TestAdminDeleteGroup_Success_Returns204(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodDelete, adminOtherPathGroups1, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestAdminDeleteGroup_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodDelete, adminOtherPathGroups1, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminDeleteGroup_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodDelete, "/groups/abc", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminDeleteGroup_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New("not found")}
	req := withCaller(newAdminRequest(http.MethodDelete, adminOtherPathGroups1, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminRemoveMember_Success_Returns204(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodDelete, "/groups/1/members/10", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestAdminRemoveMember_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodDelete, "/groups/1/members/10", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminRemoveMember_InvalidMembershipID_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodDelete, "/groups/1/members/abc", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminUpdateGroupSettings_Success_Returns200(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Liga", EntryFee: 100}
	svc := &stubAdminGroupSvc{quiniela: q}
	req := withCaller(
		newAdminRequestJSON(http.MethodPatch, adminOtherPathGroups1Settings, `{"entry_fee":100}`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminUpdateGroupSettings_MissingEntryFee_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPatch, adminOtherPathGroups1Settings, `{"max_members":10}`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminUpdateGroupSettings_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPatch, adminOtherPathGroups1Settings, `{"entry_fee":100}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminTransferOwnership_Success_Returns204(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1Transfer, `{"new_owner_user_id":5}`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestAdminTransferOwnership_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPost, adminOtherPathGroups1Transfer, `{"new_owner_user_id":5}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminTransferOwnership_InvalidNewOwner_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1Transfer, `{"new_owner_user_id":0}`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminPaymentHandler ───────────────────────────────────────────────────────

func newAdminPaymentRouter(svc *stubAdminPaymentSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminPaymentHandler(svc, zap.NewNop())
	r.Get(adminOtherPathPaymentsPending, h.ListPending)
	r.Get("/payments", h.List)
	r.Post("/payments/{id}/validate", h.ValidateDeposit)
	r.Post("/payments/{id}/reject", h.RejectDeposit)
	r.Get("/groups/{id}/payments", h.ListByGroup)
	return r
}

func TestAdminListPendingPayments_Success_Returns200(t *testing.T) {
	svc := &stubAdminPaymentSvc{records: []*domain.PaymentRecord{{ID: 1}}}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, adminOtherPathPaymentsPending, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListPendingPayments_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminPaymentSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, adminOtherPathPaymentsPending, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminListPayments_Success_Returns200(t *testing.T) {
	svc := &stubAdminPaymentSvc{records: []*domain.PaymentRecord{}}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/payments?status=pending", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListPayments_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminPaymentSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/payments", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminValidateDeposit_Success_Returns200(t *testing.T) {
	rec := &domain.PaymentRecord{ID: 1, Status: "confirmed"}
	svc := &stubAdminPaymentSvc{record: rec}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/validate", `{"notes":"ok"}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminValidateDeposit_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	w := do(newAdminPaymentRouter(svc), http.MethodPost, "/payments/1/validate", `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminValidateDeposit_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/abc/validate", `{}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminRejectDeposit_Success_Returns200(t *testing.T) {
	rec := &domain.PaymentRecord{ID: 1, Status: "rejected"}
	svc := &stubAdminPaymentSvc{record: rec}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/reject", `{"notes":"fraud"}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminRejectDeposit_MissingNotes_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/reject", `{"notes":""}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminListPaymentsByGroup_Success_Returns200(t *testing.T) {
	svc := &stubAdminPaymentSvc{records: []*domain.PaymentRecord{}}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/groups/1/payments", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListPaymentsByGroup_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/groups/abc/payments", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminLeaderboardHandler ───────────────────────────────────────────────────

func newAdminLeaderboardRouter(svc *stubAdminReadSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminLeaderboardHandler(svc, &stubAdminParamSvc{}, zap.NewNop())
	r.Get(adminOtherPathLeaderboard, h.GlobalLeaderboard)
	r.Get("/groups/{id}/leaderboard/history", h.SnapshotHistory)
	r.Get("/predictions", h.ListPredictions)
	r.Get("/predictions/match/{matchID}", h.ListPredictionsByMatch)
	return r
}

func TestAdminGlobalLeaderboard_Success_Returns200(t *testing.T) {
	svc := &stubAdminReadSvc{entries: []*domain.GlobalLeaderboardEntry{{Rank: 1, UserID: 1, TotalPoints: 50}}}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, adminOtherPathLeaderboard, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminGlobalLeaderboard_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminReadSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, adminOtherPathLeaderboard, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminSnapshotHistory_Success_Returns200(t *testing.T) {
	snap := &domain.LeaderboardSnapshot{ID: 1, QuinielaID: 1, TakenAt: time.Now()}
	svc := &stubAdminReadSvc{snapshots: []*domain.LeaderboardSnapshot{snap}}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, "/groups/1/leaderboard/history", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminSnapshotHistory_InvalidGroupID_Returns422(t *testing.T) {
	svc := &stubAdminReadSvc{}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, "/groups/abc/leaderboard/history", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminListPredictions_Success_Returns200(t *testing.T) {
	svc := &stubAdminReadSvc{preds: []*domain.Prediction{}}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, "/predictions?user_id=1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListPredictionsByMatch_Success_Returns200(t *testing.T) {
	svc := &stubAdminReadSvc{preds: []*domain.Prediction{{ID: 1}}}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, "/predictions/match/1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListPredictionsByMatch_InvalidMatchID_Returns422(t *testing.T) {
	svc := &stubAdminReadSvc{}
	w := do(newAdminLeaderboardRouter(svc), http.MethodGet, "/predictions/match/abc", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminDLQHandler ───────────────────────────────────────────────────────────

func newAdminDLQRouter(svc *stubDLQSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminDLQHandler(svc, zap.NewNop())
	r.Get(adminOtherPathDLQ, h.Stats)
	r.Post(adminOtherPathDLQReplay, h.Replay)
	r.Delete(adminOtherPathDLQ, h.Purge)
	return r
}

func TestAdminDLQStats_Success_Returns200(t *testing.T) {
	svc := &stubDLQSvc{stats: []service.DLQStat{{EventType: "match.scored", Count: 3}}}
	w := do(newAdminDLQRouter(svc), http.MethodGet, adminOtherPathDLQ, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminDLQStats_ServiceError_Returns500(t *testing.T) {
	svc := &stubDLQSvc{err: errors.New("redis down")}
	w := do(newAdminDLQRouter(svc), http.MethodGet, adminOtherPathDLQ, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminDLQReplay_Success_Returns200(t *testing.T) {
	svc := &stubDLQSvc{replayed: 5}
	w := do(newAdminDLQRouter(svc), http.MethodPost, adminOtherPathDLQReplay, `{"limit":5}`)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminDLQReplay_DefaultLimit_Returns200(t *testing.T) {
	svc := &stubDLQSvc{replayed: 0}
	w := do(newAdminDLQRouter(svc), http.MethodPost, adminOtherPathDLQReplay, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminDLQReplay_ServiceError_Returns500(t *testing.T) {
	svc := &stubDLQSvc{err: errors.New("redis error")}
	w := do(newAdminDLQRouter(svc), http.MethodPost, adminOtherPathDLQReplay, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminDLQPurge_Success_Returns200(t *testing.T) {
	svc := &stubDLQSvc{removed: 10}
	w := do(newAdminDLQRouter(svc), http.MethodDelete, adminOtherPathDLQ, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminDLQPurge_ServiceError_Returns500(t *testing.T) {
	svc := &stubDLQSvc{err: errors.New("redis error")}
	w := do(newAdminDLQRouter(svc), http.MethodDelete, adminOtherPathDLQ, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminAuditHandler ─────────────────────────────────────────────────────────

func newAdminAuditRouter(svc *stubAuditReader) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminAuditHandler(svc, zap.NewNop())
	r.Get(adminOtherPathAuditLog, h.List)
	r.Get("/audit-log/entity/{type}/{id}", h.ListByEntity)
	return r
}

func TestAdminAuditList_Success_Returns200(t *testing.T) {
	svc := &stubAuditReader{logs: []*domain.AuditLog{{ID: 1, Action: "user.ban"}}}
	w := do(newAdminAuditRouter(svc), http.MethodGet, adminOtherPathAuditLog, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminAuditList_WithFilters_Returns200(t *testing.T) {
	svc := &stubAuditReader{logs: []*domain.AuditLog{}}
	w := do(newAdminAuditRouter(svc), http.MethodGet, "/audit-log?action=user.ban&resource_type=user&actor_id=1", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminAuditList_ServiceError_Returns500(t *testing.T) {
	svc := &stubAuditReader{err: errors.New(adminOtherDBError)}
	w := do(newAdminAuditRouter(svc), http.MethodGet, adminOtherPathAuditLog, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminAuditListByEntity_Success_Returns200(t *testing.T) {
	svc := &stubAuditReader{logs: []*domain.AuditLog{}}
	w := do(newAdminAuditRouter(svc), http.MethodGet, "/audit-log/entity/user/5", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminAuditListByEntity_InvalidID_Returns422(t *testing.T) {
	svc := &stubAuditReader{}
	w := do(newAdminAuditRouter(svc), http.MethodGet, "/audit-log/entity/user/abc", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminAuditListByEntity_ServiceError_Returns500(t *testing.T) {
	svc := &stubAuditReader{err: errors.New(adminOtherDBError)}
	w := do(newAdminAuditRouter(svc), http.MethodGet, "/audit-log/entity/user/5", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminSystemParamHandler ───────────────────────────────────────────────────

func newAdminParamRouter(svc *stubAdminParamSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminSystemParamHandler(svc, zap.NewNop())
	r.Get(adminOtherPathSysParams, h.ListAll)
	r.Get("/system-params/{key}", h.Get)
	r.Patch("/system-params/{key}", h.Set)
	r.Post(adminOtherPathSysParamsBulk, h.BulkSet)
	return r
}

func TestAdminParamListAll_Success_Returns200(t *testing.T) {
	p := &domain.SystemParam{Key: adminOtherScoringExact, Value: "5"}
	svc := &stubAdminParamSvc{params: []*domain.SystemParam{p}}
	w := do(newAdminParamRouter(svc), http.MethodGet, adminOtherPathSysParams, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamListAll_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminParamSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminParamRouter(svc), http.MethodGet, adminOtherPathSysParams, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminParamGet_Success_Returns200(t *testing.T) {
	p := &domain.SystemParam{Key: adminOtherScoringExact, Value: "5"}
	svc := &stubAdminParamSvc{param: p}
	w := do(newAdminParamRouter(svc), http.MethodGet, "/system-params/scoring.exact", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamGet_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminParamSvc{err: errors.New("not found")}
	w := do(newAdminParamRouter(svc), http.MethodGet, "/system-params/missing.key", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminParamSet_Success_Returns200(t *testing.T) {
	p := &domain.SystemParam{Key: adminOtherScoringExact, Value: "10"}
	svc := &stubAdminParamSvc{param: p}
	req := withCaller(
		newAdminRequestJSON(http.MethodPatch, "/system-params/scoring.exact", `{"value":"10"}`),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamSet_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminParamSvc{}
	w := do(newAdminParamRouter(svc), http.MethodPatch, "/system-params/k", `{"value":"v"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminParamSet_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminParamSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPatch, "/system-params/k", `not-json`),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminParamBulkSet_Success_Returns204(t *testing.T) {
	svc := &stubAdminParamSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathSysParamsBulk, `{"params":{"k":"v"}}`),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestAdminParamBulkSet_EmptyParams_Returns422(t *testing.T) {
	svc := &stubAdminParamSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathSysParamsBulk, `{"params":{}}`),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminParamBulkSet_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminParamSvc{}
	w := do(newAdminParamRouter(svc), http.MethodPost, adminOtherPathSysParamsBulk, `{"params":{"k":"v"}}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

// ── AdminTiebreakerHandler ────────────────────────────────────────────────────

func newAdminTiebreakerRouter(svc *stubAdminReadSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminTiebreakerHandler(svc, zap.NewNop())
	r.Get(adminOtherPathTiebreakerSubs, h.ListSubmissions)
	return r
}

func TestAdminListTiebreakerSubmissions_Success_Returns200(t *testing.T) {
	tb := &domain.Tiebreaker{ID: 1, UserID: 1, Prediction: 3}
	views := []service.TiebreakerSubmissionView{{Submission: tb, UserName: "alice"}}
	svc := &stubAdminReadSvc{views: views}
	w := do(newAdminTiebreakerRouter(svc), http.MethodGet, adminOtherPathTiebreakerSubs, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListTiebreakerSubmissions_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminReadSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminTiebreakerRouter(svc), http.MethodGet, adminOtherPathTiebreakerSubs, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminConflictHandler ──────────────────────────────────────────────────────

func newAdminConflictRouter(svc *stubConflictSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminConflictHandler(svc, zap.NewNop())
	r.Get(adminOtherPathConflicts, h.ListConflicts)
	r.Post("/conflicts/{type}/{id}/resolve", h.ResolveConflict)
	return r
}

func TestAdminListConflicts_Success_Returns200(t *testing.T) {
	svc := &stubConflictSvc{conflicts: []domain.Conflict{{Type: domain.ConflictGroupNoOwner, EntityID: 1, EntityType: "quiniela"}}}
	w := do(newAdminConflictRouter(svc), http.MethodGet, adminOtherPathConflicts, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminListConflicts_ServiceError_Returns500(t *testing.T) {
	svc := &stubConflictSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminConflictRouter(svc), http.MethodGet, adminOtherPathConflicts, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminResolveConflict_Success_Returns204(t *testing.T) {
	svc := &stubConflictSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathConflictResolve, `{"note":"transferred ownership"}`),
		adminCaller,
	)
	w := doReq(newAdminConflictRouter(svc), req)
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestAdminResolveConflict_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubConflictSvc{}
	w := do(newAdminConflictRouter(svc), http.MethodPost, adminOtherPathConflictResolve, `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminResolveConflict_InvalidEntityID_Returns422(t *testing.T) {
	svc := &stubConflictSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/conflicts/group_without_owner/abc/resolve", `{}`),
		adminCaller,
	)
	w := doReq(newAdminConflictRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminResolveConflict_ServiceError_Returns500(t *testing.T) {
	svc := &stubConflictSvc{err: errors.New("audit fail")}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathConflictResolve, `{}`),
		adminCaller,
	)
	w := doReq(newAdminConflictRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
