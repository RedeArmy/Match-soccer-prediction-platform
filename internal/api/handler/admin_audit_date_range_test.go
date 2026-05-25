package handler_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

func newAuditRouterDateRange(svc *stubAuditReader) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminAuditHandler(svc, zap.NewNop())
	r.Get("/audit-log", h.List)
	return r
}

func TestAdminAuditList_DateRange_ValidRFC3339(t *testing.T) {
	svc := &stubAuditReader{}
	url := "/audit-log?created_after=2024-01-01T00:00:00Z&created_before=2024-12-31T23:59:59Z"
	w := do(newAuditRouterDateRange(svc), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid date range, got %d", w.Code)
	}
}

func TestAdminAuditList_DateRange_InvalidCreatedAfter(t *testing.T) {
	svc := &stubAuditReader{}
	w := do(newAuditRouterDateRange(svc), http.MethodGet, "/audit-log?created_after=not-a-date", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid created_after, got %d", w.Code)
	}
}

func TestAdminAuditList_DateRange_InvalidCreatedBefore(t *testing.T) {
	svc := &stubAuditReader{}
	w := do(newAuditRouterDateRange(svc), http.MethodGet, "/audit-log?created_before=not-a-date", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid created_before, got %d", w.Code)
	}
}

func TestAdminAuditList_DateRange_OnlyAfter(t *testing.T) {
	svc := &stubAuditReader{}
	w := do(newAuditRouterDateRange(svc), http.MethodGet, "/audit-log?created_after=2024-06-01T00:00:00Z", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with only created_after, got %d", w.Code)
	}
}
