package handler_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── stub repository ───────────────────────────────────────────────────────────

type stubNotifDLQRepo struct {
	count      int64
	countErr   error
	recent     []*domain.NotificationDLQEntry
	recentErr  error
	resolveErr error
}

func (s *stubNotifDLQRepo) CreateEntry(_ context.Context, _ *domain.NotificationDLQEntry) error {
	return nil
}
func (s *stubNotifDLQRepo) ClaimBatch(_ context.Context, _, _ int) ([]*domain.NotificationDLQEntry, error) {
	return nil, nil
}
func (s *stubNotifDLQRepo) MarkResolved(_ context.Context, _ int64) error            { return s.resolveErr }
func (s *stubNotifDLQRepo) RecordFailure(_ context.Context, _ int64, _ string) error { return nil }
func (s *stubNotifDLQRepo) CountUnresolved(_ context.Context) (int64, error) {
	return s.count, s.countErr
}
func (s *stubNotifDLQRepo) ListRecent(_ context.Context, _ int) ([]*domain.NotificationDLQEntry, error) {
	return s.recent, s.recentErr
}

// ── router factory ────────────────────────────────────────────────────────────

func newNotifDLQRouter(repo *stubNotifDLQRepo) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminNotificationDLQHandler(repo, zap.NewNop())
	r.Get("/notification-dlq", h.Stats)
	r.Post("/notification-dlq/{id}/resolve", h.Resolve)
	return r
}

func dlqEntryFix() *domain.NotificationDLQEntry {
	now := time.Now()
	return &domain.NotificationDLQEntry{
		ID:          1,
		Channel:     "email",
		EventType:   "admin.bank_transfer_pending",
		ErrorDetail: "smtp timeout",
		Attempts:    1,
		CreatedAt:   now,
	}
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func TestNotifDLQStats_Success_Returns200(t *testing.T) {
	repo := &stubNotifDLQRepo{count: 3, recent: []*domain.NotificationDLQEntry{dlqEntryFix()}}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifDLQStats_EmptyQueue_Returns200(t *testing.T) {
	repo := &stubNotifDLQRepo{count: 0, recent: []*domain.NotificationDLQEntry{}}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifDLQStats_WithLimitParam_Returns200(t *testing.T) {
	repo := &stubNotifDLQRepo{count: 1, recent: []*domain.NotificationDLQEntry{dlqEntryFix()}}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq?limit=50", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestNotifDLQStats_CountUnresolvedError_Returns500(t *testing.T) {
	repo := &stubNotifDLQRepo{countErr: errors.New("db error")}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestNotifDLQStats_ListRecentError_Returns500(t *testing.T) {
	repo := &stubNotifDLQRepo{count: 0, recentErr: errors.New("db error")}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestNotifDLQStats_EntryWithResolvedAt_ShowsResolved(t *testing.T) {
	now := time.Now()
	entry := dlqEntryFix()
	entry.ResolvedAt = &now
	repo := &stubNotifDLQRepo{count: 0, recent: []*domain.NotificationDLQEntry{entry}}
	w := do(newNotifDLQRouter(repo), http.MethodGet, "/notification-dlq", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

// ── Resolve ───────────────────────────────────────────────────────────────────

func TestNotifDLQResolve_Success_Returns204(t *testing.T) {
	repo := &stubNotifDLQRepo{}
	w := do(newNotifDLQRouter(repo), http.MethodPost, "/notification-dlq/42/resolve", "")
	if w.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, w.Code)
	}
}

func TestNotifDLQResolve_InvalidID_Returns422(t *testing.T) {
	repo := &stubNotifDLQRepo{}
	w := do(newNotifDLQRouter(repo), http.MethodPost, "/notification-dlq/abc/resolve", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifDLQResolve_ZeroID_Returns422(t *testing.T) {
	repo := &stubNotifDLQRepo{}
	w := do(newNotifDLQRouter(repo), http.MethodPost, "/notification-dlq/0/resolve", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestNotifDLQResolve_RepoError_Returns500(t *testing.T) {
	repo := &stubNotifDLQRepo{resolveErr: errors.New("db error")}
	w := do(newNotifDLQRouter(repo), http.MethodPost, "/notification-dlq/1/resolve", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
