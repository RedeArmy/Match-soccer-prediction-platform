package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// withCaller injects a *domain.User into the request context so that
// middleware.UserFromContext succeeds in handlers that require an authenticated caller.
func withCaller(r *http.Request, u *domain.User) *http.Request {
	return r.WithContext(middleware.ContextWithUser(r.Context(), u))
}

// adminCaller is the default admin user injected into requests under test.
var adminCaller = &domain.User{ID: 99, Name: "admin", Role: domain.RoleAdmin}

// newAdminRequest builds a plain HTTP request (no Content-Type header).
func newAdminRequest(method, path, body string) *http.Request {
	var b *strings.Reader
	if body != "" {
		b = strings.NewReader(body)
	} else {
		b = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, path, b)
	return req
}

// newAdminRequestJSON builds an HTTP request with Content-Type: application/json.
func newAdminRequestJSON(method, path, body string) *http.Request {
	req := newAdminRequest(method, path, body)
	req.Header.Set(headerContentType, contentTypeJSON)
	return req
}

// doReq dispatches a pre-built request through the given router.
func doReq(router http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── AdminUserService stub ─────────────────────────────────────────────────────

type stubAdminUserSvc struct {
	user       *domain.User
	users      []*domain.User
	profile    *service.AdminUserProfile
	bulkResult service.BulkBanResult
	err        error
}

func (s *stubAdminUserSvc) BanUser(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return s.user, s.err
}
func (s *stubAdminUserSvc) UnbanUser(_ context.Context, _, _ int) (*domain.User, error) {
	return s.user, s.err
}
func (s *stubAdminUserSvc) ListUsers(_ context.Context) ([]*domain.User, error) {
	return s.users, s.err
}
func (s *stubAdminUserSvc) BulkBan(_ context.Context, _ []int, _ int, _ string) (service.BulkBanResult, error) {
	if s.err != nil {
		return service.BulkBanResult{}, s.err
	}
	return s.bulkResult, nil
}
func (s *stubAdminUserSvc) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.Pagination) ([]*domain.User, error) {
	return s.users, s.err
}
func (s *stubAdminUserSvc) GetProfile(_ context.Context, _ int) (*service.AdminUserProfile, error) {
	return s.profile, s.err
}

// ── AdminGroupService stub ────────────────────────────────────────────────────

type stubAdminGroupSvc struct {
	quiniela *domain.Quiniela
	err      error
}

func (s *stubAdminGroupSvc) DeleteGroup(_ context.Context, _, _ int) error  { return s.err }
func (s *stubAdminGroupSvc) RemoveMember(_ context.Context, _, _ int) error { return s.err }
func (s *stubAdminGroupSvc) UpdateGroupSettings(_ context.Context, _, _, _ int) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubAdminGroupSvc) TransferOwnership(_ context.Context, _, _, _ int) error { return s.err }
func (s *stubAdminGroupSvc) BulkDeleteGroups(_ context.Context, ids []int, _ int) (service.BulkOperationResult, error) {
	if s.err != nil {
		return service.BulkOperationResult{}, s.err
	}
	return service.BulkOperationResult{Succeeded: ids, Failed: []int{}}, nil
}
func (s *stubAdminGroupSvc) BulkRemoveMembers(_ context.Context, _ int, ids []int, _ int) (service.BulkOperationResult, error) {
	if s.err != nil {
		return service.BulkOperationResult{}, s.err
	}
	return service.BulkOperationResult{Succeeded: ids}, nil
}
func (s *stubAdminGroupSvc) RecalculateLeaderboard(_ context.Context, quinielaID, _ int) (*domain.LeaderboardSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.LeaderboardSnapshot{QuinielaID: quinielaID}, nil
}

// ── AdminPaymentService stub (extends stubPaymentSvc) ────────────────────────

type stubAdminPaymentSvc struct {
	record  *domain.PaymentRecord
	records []*domain.PaymentRecord
	err     error
}

func (s *stubAdminPaymentSvc) CreateRecord(_ context.Context, _, _, _ int, _, _ string) (*domain.PaymentRecord, error) {
	return s.record, s.err
}
func (s *stubAdminPaymentSvc) ValidateDeposit(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return s.record, s.err
}
func (s *stubAdminPaymentSvc) RejectDeposit(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return s.record, s.err
}
func (s *stubAdminPaymentSvc) ListPending(_ context.Context) ([]*domain.PaymentRecord, error) {
	return s.records, s.err
}
func (s *stubAdminPaymentSvc) ListByQuiniela(_ context.Context, _ int) ([]*domain.PaymentRecord, error) {
	return s.records, s.err
}
func (s *stubAdminPaymentSvc) List(_ context.Context, _ repository.PaymentFilters, _ repository.Pagination) ([]*domain.PaymentRecord, error) {
	return s.records, s.err
}

// ── AdminReadService stub ─────────────────────────────────────────────────────

type stubAdminReadSvc struct {
	entries   []*domain.GlobalLeaderboardEntry
	preds     []*domain.Prediction
	views     []service.TiebreakerSubmissionView
	snapshots []*domain.LeaderboardSnapshot
	err       error
}

func (s *stubAdminReadSvc) GlobalLeaderboard(_ context.Context, _ int) ([]*domain.GlobalLeaderboardEntry, error) {
	return s.entries, s.err
}
func (s *stubAdminReadSvc) ListPredictions(_ context.Context, _ repository.PredictionAdminFilters, _ repository.Pagination) ([]*domain.Prediction, error) {
	return s.preds, s.err
}
func (s *stubAdminReadSvc) ListTiebreakerSubmissions(_ context.Context, _ repository.Pagination) ([]service.TiebreakerSubmissionView, error) {
	return s.views, s.err
}
func (s *stubAdminReadSvc) ListSnapshotHistory(_ context.Context, _, _ int) ([]*domain.LeaderboardSnapshot, error) {
	return s.snapshots, s.err
}
func (s *stubAdminReadSvc) GetDashboardStats(_ context.Context) (*domain.DashboardStats, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.DashboardStats{}, nil
}

// ── DLQService stub ───────────────────────────────────────────────────────────

type stubDLQSvc struct {
	stats    []service.DLQStat
	replayed int
	removed  int64
	err      error
}

func (s *stubDLQSvc) Stats(_ context.Context) ([]service.DLQStat, error) {
	return s.stats, s.err
}
func (s *stubDLQSvc) Replay(_ context.Context, _ int) (int, error) {
	return s.replayed, s.err
}
func (s *stubDLQSvc) Purge(_ context.Context) (int64, error) {
	return s.removed, s.err
}

// ── AuditReader stub ──────────────────────────────────────────────────────────

type stubAuditReader struct {
	logs []*domain.AuditLog
	err  error
}

func (s *stubAuditReader) ListAuditLogs(_ context.Context, _ repository.AuditLogFilters, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return s.logs, s.err
}
func (s *stubAuditReader) ListAuditLogsByEntity(_ context.Context, _ string, _ int, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return s.logs, s.err
}

// ── SystemParamService stub (admin-scoped) ────────────────────────────────────

type stubAdminParamSvc struct {
	param  *domain.SystemParam
	params []*domain.SystemParam
	err    error
}

func (s *stubAdminParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return s.param, s.err
}
func (s *stubAdminParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return s.params, s.err
}
func (s *stubAdminParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return s.params, s.err
}
func (s *stubAdminParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return s.param, s.err
}
func (s *stubAdminParamSvc) GetString(_ context.Context, _ string, d string) string { return d }
func (s *stubAdminParamSvc) GetInt(_ context.Context, _ string, d int) int          { return d }
func (s *stubAdminParamSvc) GetDuration(_ context.Context, _ string, d time.Duration) time.Duration {
	return d
}
func (s *stubAdminParamSvc) GetBool(_ context.Context, _ string, d bool) bool { return d }
func (s *stubAdminParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return s.err
}
func (s *stubAdminParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return s.param, s.err
}

// ── ConflictService stub ──────────────────────────────────────────────────────

type stubConflictSvc struct {
	conflicts []domain.Conflict
	summary   *service.ConflictSummaryResult
	err       error
}

func (s *stubConflictSvc) ListConflicts(_ context.Context, _ repository.Pagination) ([]domain.Conflict, error) {
	return s.conflicts, s.err
}
func (s *stubConflictSvc) ConflictSummary(_ context.Context) (*service.ConflictSummaryResult, error) {
	return s.summary, s.err
}
func (s *stubConflictSvc) ResolveConflict(_ context.Context, _ string, _, _ int, _, _ string) error {
	return s.err
}

// ── ScoringRuleService stub ───────────────────────────────────────────────────

type stubScoringRuleSvc struct {
	rule  *domain.ScoringRule
	rules []*domain.ScoringRule
	err   error
}

func (s *stubScoringRuleSvc) List(_ context.Context) ([]*domain.ScoringRule, error) {
	return s.rules, s.err
}
func (s *stubScoringRuleSvc) GetByPhase(_ context.Context, _ domain.MatchPhase) (*domain.ScoringRule, error) {
	return s.rule, s.err
}
func (s *stubScoringRuleSvc) Update(_ context.Context, _ domain.MatchPhase, _ service.ScoringRuleInput, _ int) (*domain.ScoringRule, error) {
	return s.rule, s.err
}
