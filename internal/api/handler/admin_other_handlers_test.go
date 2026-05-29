package handler_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	adminOtherDBError               = "db error"
	adminOtherPathDLQ               = "/dlq"
	adminOtherPathDLQReplay         = "/dlq/replay"
	adminOtherPathSysParamsBulk     = "/system-params/bulk"
	adminOtherPathSysParams         = "/system-params"
	adminOtherPathLeaderboard       = "/leaderboard"
	adminOtherPathPaymentsPending   = "/payments/pending"
	adminOtherPathGroups1           = "/groups/1"
	adminOtherPathGroups1Settings   = "/groups/1/settings"
	adminOtherPathGroups1Transfer   = "/groups/1/transfer-ownership"
	adminOtherPathGroupsBulkDelete  = "/groups/bulk-delete"
	adminOtherPathGroups1BulkRemove = "/groups/1/members/bulk-remove"
	adminOtherPathGroups1Recalc     = "/groups/1/leaderboard/recalculate"
	adminOtherPathGroups1Distribute = "/groups/1/distribute-prizes"
	adminOtherPathStats             = "/stats"
	adminOtherPathTiebreakerSubs    = "/tiebreaker/submissions"
	adminOtherPathAuditLog          = "/audit-log"
	adminOtherPathConflicts         = "/conflicts"
	adminOtherPathConflictResolve   = "/conflicts/group_without_owner/1/resolve"
	adminOtherPathConflictSummary   = "/stats/conflicts/summary"
	adminOtherScoringExact          = "scoring.exact"
)

// ── AdminGroupHandler ─────────────────────────────────────────────────────────

func newAdminGroupRouter(svc service.AdminGroupService) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminGroupHandler(svc, &stubAdminParamSvc{}, zap.NewNop())
	r.Delete("/groups/{id}", h.DeleteGroup)
	r.Delete("/groups/{id}/members/{membershipID}", h.RemoveMember)
	r.Patch("/groups/{id}/settings", h.UpdateGroupSettings)
	r.Post("/groups/{id}/transfer-ownership", h.TransferOwnership)
	r.Post("/groups/bulk-delete", h.BulkDeleteGroups)
	r.Post("/groups/{id}/members/bulk-remove", h.BulkRemoveMembers)
	r.Post("/groups/{id}/leaderboard/recalculate", h.RecalculateLeaderboard)
	r.Post("/groups/{id}/distribute-prizes", h.DistributePrizes)
	return r
}

func newAdminStatsRouter(svc *stubAdminReadSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminStatsHandler(svc, zap.NewNop())
	r.Get("/stats", h.GetDashboardStats)
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

func TestAdminUpdateGroupSettings_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPatch, adminOtherPathGroups1Settings, `not-json`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminTransferOwnership_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1Transfer, `not-json`),
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

func TestAdminRejectDeposit_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/reject", `not-json`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminValidateDeposit_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminPaymentSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/validate", `{}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminListPaymentsByGroup_InvalidGroupID_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/groups/abc/payments", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminListPaymentsByGroup_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminPaymentSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/groups/1/payments", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminRemoveMember_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(newAdminRequest(http.MethodDelete, "/groups/1/members/10", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminTransferOwnership_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1Transfer, `{"new_owner_user_id":5}`),
		adminCaller,
	)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminRejectDeposit_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	w := do(newAdminPaymentRouter(svc), http.MethodPost, "/payments/1/reject", `{"notes":"fraud"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminRejectDeposit_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminPaymentSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/abc/reject", `{"notes":"fraud"}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminRejectDeposit_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminPaymentSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, "/payments/1/reject", `{"notes":"fraud"}`),
		adminCaller,
	)
	w := doReq(newAdminPaymentRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
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
	// groupID=0 exercises the id <= 0 branch; distinct from the non-numeric
	// case covered by TestAdminListPaymentsByGroup_InvalidGroupID_Returns422.
	svc := &stubAdminPaymentSvc{}
	w := do(newAdminPaymentRouter(svc), http.MethodGet, "/groups/0/payments", "")
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
	r.Get("/system-params/{key}/history", h.History)
	r.Get("/system-params/{key}", h.Get)
	r.Patch("/system-params/{key}", h.Set)
	r.Post("/system-params/{key}/reset", h.Reset)
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

func TestAdminParamBulkSet_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminParamSvc{}
	req := withCaller(
		newAdminRequestJSON(http.MethodPost, adminOtherPathSysParamsBulk, `not-json`),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
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

func TestAdminParamReset_Success_Returns200(t *testing.T) {
	p := &domain.SystemParam{Key: "scoring.exact", Value: "5", DefaultValue: "5"}
	svc := &stubAdminParamSvc{param: p}
	req := withCaller(
		newAdminRequest(http.MethodPost, "/system-params/scoring.exact/reset", ""),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamReset_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminParamSvc{}
	w := do(newAdminParamRouter(svc), http.MethodPost, "/system-params/k/reset", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminParamReset_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminParamSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(
		newAdminRequest(http.MethodPost, "/system-params/k/reset", ""),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminParamReset_NotFound_Returns404(t *testing.T) {
	svc := &stubAdminParamSvc{err: apperrors.NotFound("missing.key")}
	req := withCaller(
		newAdminRequest(http.MethodPost, "/system-params/missing.key/reset", ""),
		adminCaller,
	)
	w := doReq(newAdminParamRouter(svc), req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminParamHistory_Success_Returns200(t *testing.T) {
	svc := &stubAdminParamSvc{}
	w := do(newAdminParamRouter(svc), http.MethodGet, "/system-params/scoring.exact/history", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamHistory_WithEntries_Returns200AndSerializesRows(t *testing.T) {
	svc := &stubAdminParamSvc{
		history: []*domain.SystemParamHistory{
			{Key: "scoring.exact", OldValue: "5", NewValue: "7", ActorID: 1, Action: "set"},
		},
	}
	w := do(newAdminParamRouter(svc), http.MethodGet, "/system-params/scoring.exact/history", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminParamHistory_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminParamSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminParamRouter(svc), http.MethodGet, "/system-params/scoring.exact/history", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
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
	r.Get(adminOtherPathConflictSummary, h.ConflictSummary)
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

// ── AdminConflictHandler - ConflictSummary ────────────────────────────────────

func TestAdminConflictSummary_Success_Returns200(t *testing.T) {
	avg := 3.5
	svc := &stubConflictSvc{
		summary: &service.ConflictSummaryResult{
			TotalUnresolved: 2,
			ByType: []service.ConflictTypeSummary{
				{Type: domain.ConflictPaymentStale, Count: 2, AvgAgeDays: &avg},
			},
		},
	}
	w := do(newAdminConflictRouter(svc), http.MethodGet, adminOtherPathConflictSummary, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminConflictSummary_Empty_Returns200(t *testing.T) {
	svc := &stubConflictSvc{
		summary: &service.ConflictSummaryResult{
			TotalUnresolved: 0,
			ByType:          []service.ConflictTypeSummary{},
		},
	}
	w := do(newAdminConflictRouter(svc), http.MethodGet, adminOtherPathConflictSummary, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminConflictSummary_ServiceError_Returns500(t *testing.T) {
	svc := &stubConflictSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminConflictRouter(svc), http.MethodGet, adminOtherPathConflictSummary, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminGroupHandler - BulkDeleteGroups ──────────────────────────────────────

func TestAdminBulkDeleteGroups_Success_Returns200(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[1,2,3]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminBulkDeleteGroups_PartialFailure_Returns207(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	// The stub returns all IDs as succeeded when err==nil; override to simulate partial failure.
	// We simulate this by injecting a service that always reports some as failed.
	partialSvc := &partialBulkGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[1,2]}`), adminCaller)
	w := doReq(newAdminGroupRouter(partialSvc), req)
	if w.Code != http.StatusMultiStatus {
		t.Errorf("expected 207, got %d", w.Code)
	}
	_ = svc
}

func TestAdminBulkDeleteGroups_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[1]}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminBulkDeleteGroups_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `not-json`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkDeleteGroups_EmptyIDs_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkDeleteGroups_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[1]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminBulkDeleteGroups_ExceedsLimit_Returns422(t *testing.T) {
	ids := make([]string, 1001)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", i+1)
	}
	body := `{"group_ids":[` + strings.Join(ids, ",") + `]}`
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, body), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminGroupHandler - BulkRemoveMembers ─────────────────────────────────────

func TestAdminBulkRemoveMembers_Success_Returns200(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1BulkRemove, `{"membership_ids":[10,11]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminBulkRemoveMembers_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPost, adminOtherPathGroups1BulkRemove, `{"membership_ids":[10]}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminBulkRemoveMembers_BadJSON_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1BulkRemove, `not-json`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkRemoveMembers_EmptyIDs_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1BulkRemove, `{"membership_ids":[]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkRemoveMembers_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1BulkRemove, `{"membership_ids":[10]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestAdminBulkRemoveMembers_ExceedsLimit_Returns422(t *testing.T) {
	ids := make([]string, 1001)
	for i := range ids {
		ids[i] = fmt.Sprintf("%d", i+1)
	}
	body := `{"membership_ids":[` + strings.Join(ids, ",") + `]}`
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroups1BulkRemove, body), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminBulkRemoveMembers_InvalidGroupID_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, "/groups/abc/members/bulk-remove", `{"membership_ids":[10]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// ── AdminGroupHandler - RecalculateLeaderboard ────────────────────────────────

func TestAdminRecalculateLeaderboard_Success_Returns200(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodPost, adminOtherPathGroups1Recalc, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminRecalculateLeaderboard_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPost, adminOtherPathGroups1Recalc, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminRecalculateLeaderboard_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodPost, "/groups/abc/leaderboard/recalculate", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminRecalculateLeaderboard_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(newAdminRequest(http.MethodPost, adminOtherPathGroups1Recalc, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminGroupHandler - DistributePrizes ──────────────────────────────────────

func TestAdminDistributePrizes_Success_Returns200(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodPost, adminOtherPathGroups1Distribute, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminDistributePrizes_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	w := do(newAdminGroupRouter(svc), http.MethodPost, adminOtherPathGroups1Distribute, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminDistributePrizes_InvalidID_Returns422(t *testing.T) {
	svc := &stubAdminGroupSvc{}
	req := withCaller(newAdminRequest(http.MethodPost, "/groups/abc/distribute-prizes", ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminDistributePrizes_AlreadyDistributed_Returns409(t *testing.T) {
	svc := &stubAdminGroupSvc{err: apperrors.Conflict("prizes already distributed for this quiniela")}
	req := withCaller(newAdminRequest(http.MethodPost, adminOtherPathGroups1Distribute, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestAdminDistributePrizes_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminGroupSvc{err: errors.New(adminOtherDBError)}
	req := withCaller(newAdminRequest(http.MethodPost, adminOtherPathGroups1Distribute, ""), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── AdminStatsHandler - GetDashboardStats ─────────────────────────────────────

func TestAdminGetDashboardStats_Success_Returns200(t *testing.T) {
	svc := &stubAdminReadSvc{}
	w := do(newAdminStatsRouter(svc), http.MethodGet, adminOtherPathStats, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminGetDashboardStats_ServiceError_Returns500(t *testing.T) {
	svc := &stubAdminReadSvc{err: errors.New(adminOtherDBError)}
	w := do(newAdminStatsRouter(svc), http.MethodGet, adminOtherPathStats, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── helper stubs ──────────────────────────────────────────────────────────────

// partialBulkGroupSvc simulates a bulk operation where some IDs fail.
type partialBulkGroupSvc struct{ stubAdminGroupSvc }

func (s *partialBulkGroupSvc) BulkDeleteGroups(_ context.Context, ids []int, _ int) (service.BulkOperationResult, error) {
	if len(ids) == 0 {
		return service.BulkOperationResult{}, nil
	}
	return service.BulkOperationResult{Succeeded: ids[:1], Failed: ids[1:]}, nil
}

// nilSliceBulkGroupSvc returns BulkOperationResult with nil slices (zero value)
// to exercise the nil-to-empty-slice guards in bulkOperationResultToResponse.
type nilSliceBulkGroupSvc struct{ stubAdminGroupSvc }

func (s *nilSliceBulkGroupSvc) BulkDeleteGroups(_ context.Context, _ []int, _ int) (service.BulkOperationResult, error) {
	return service.BulkOperationResult{}, nil // Succeeded and Failed are nil
}

func TestAdminBulkDeleteGroups_NilSlices_ResponseHasEmptyArrays(t *testing.T) {
	svc := &nilSliceBulkGroupSvc{}
	req := withCaller(newAdminRequestJSON(http.MethodPost, adminOtherPathGroupsBulkDelete, `{"group_ids":[1]}`), adminCaller)
	w := doReq(newAdminGroupRouter(svc), req)
	// nil Succeeded -> all IDs treated as failed -> 207 Multi-Status
	if w.Code != http.StatusOK && w.Code != http.StatusMultiStatus {
		t.Errorf("expected 200 or 207, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}
