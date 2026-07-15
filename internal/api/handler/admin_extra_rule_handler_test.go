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

// newExtraRuleRouter wires the handler under test to an isolated chi router.
func newExtraRuleRouter(svc *stubExtraRuleSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminExtraRuleHandler(svc, zap.NewNop())
	r.Get("/extra-rules", h.List)
	r.Get("/extra-rules/{extraType}", h.GetByType)
	r.Patch("/extra-rules/{extraType}", h.Update)
	return r
}

// extraRuleFixtureHandler returns a minimal ExtraRule for handler tests.
func extraRuleFixtureHandler() *domain.ExtraRule {
	return &domain.ExtraRule{
		ID:        1,
		ExtraType: domain.ExtraTypeFirstScorer,
		Points:    domain.DefaultExtraFirstScorerPoints,
		IsActive:  true,
		UpdatedAt: time.Now(),
	}
}

const (
	pathExtraRules             = "/extra-rules"
	pathExtraRulesFirstScorer  = "/extra-rules/first_scorer"
	pathExtraRulesHalftimeType = "/extra-rules/halftime_result"
)

// ── List ──────────────────────────────────────────────────────────────────────

func TestAdminExtraRuleList_Success_Returns200(t *testing.T) {
	svc := &stubExtraRuleSvc{rules: []*domain.ExtraRule{extraRuleFixtureHandler()}}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRules, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminExtraRuleList_EmptyRules_Returns200(t *testing.T) {
	svc := &stubExtraRuleSvc{rules: []*domain.ExtraRule{}}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRules, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminExtraRuleList_ServiceError_Returns500(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.Internal(nil)}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRules, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── GetByType ─────────────────────────────────────────────────────────────────

func TestAdminExtraRuleGetByType_Success_Returns200(t *testing.T) {
	svc := &stubExtraRuleSvc{rule: extraRuleFixtureHandler()}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRulesFirstScorer, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminExtraRuleGetByType_NotFound_Returns404(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.NotFound("extra type not found")}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRulesFirstScorer, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminExtraRuleGetByType_ServiceError_Returns500(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.Internal(nil)}
	w := do(newExtraRuleRouter(svc), http.MethodGet, pathExtraRulesFirstScorer, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestAdminExtraRuleUpdate_Success_Returns200(t *testing.T) {
	svc := &stubExtraRuleSvc{rule: extraRuleFixtureHandler()}
	body := `{"points":5,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesFirstScorer, body), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminExtraRuleUpdate_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubExtraRuleSvc{rule: extraRuleFixtureHandler()}
	body := `{"points":5,"is_active":true}`
	w := do(newExtraRuleRouter(svc), http.MethodPatch, pathExtraRulesFirstScorer, body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminExtraRuleUpdate_InvalidJSON_Returns400(t *testing.T) {
	svc := &stubExtraRuleSvc{rule: extraRuleFixtureHandler()}
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesFirstScorer, "not-json"), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminExtraRuleUpdate_ValidationError_Returns422(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.Validation("points must be non-negative")}
	body := `{"points":-1,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesFirstScorer, body), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminExtraRuleUpdate_TypeNotFound_Returns404(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.NotFound("extra rule not found for type")}
	body := `{"points":5,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesFirstScorer, body), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminExtraRuleUpdate_HalftimeResultType_Success_Returns200(t *testing.T) {
	rule := extraRuleFixtureHandler()
	rule.ExtraType = domain.ExtraTypeHalftimeResult
	rule.Points = 2
	svc := &stubExtraRuleSvc{rule: rule}
	body := `{"points":2,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesHalftimeType, body), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminExtraRuleUpdate_ServiceError_Returns500(t *testing.T) {
	svc := &stubExtraRuleSvc{err: apperrors.Internal(nil)}
	body := `{"points":5,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathExtraRulesFirstScorer, body), adminCaller)
	w := doReq(newExtraRuleRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
