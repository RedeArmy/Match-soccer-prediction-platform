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

// newScoringRuleRouter wires the handler under test to an isolated chi router.
func newScoringRuleRouter(svc *stubScoringRuleSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminScoringRuleHandler(svc, zap.NewNop())
	r.Get("/scoring-rules", h.List)
	r.Get("/scoring-rules/{phase}", h.GetByPhase)
	r.Patch("/scoring-rules/{phase}", h.Update)
	return r
}

// scoringRuleFixtureHandler returns a minimal ScoringRule for handler tests.
func scoringRuleFixtureHandler() *domain.ScoringRule {
	return &domain.ScoringRule{
		ID:             1,
		Phase:          domain.PhaseGroupStage,
		ExactScore:     5,
		CorrectOutcome: 2,
		GoalDifference: 1,
		IsActive:       true,
		UpdatedAt:      time.Now(),
	}
}

const (
	pathScoringRules           = "/scoring-rules"
	pathScoringRulesGroupStage = "/scoring-rules/group_stage"
	pathScoringRulesFinal      = "/scoring-rules/final"
)

// ── List ──────────────────────────────────────────────────────────────────────

func TestAdminScoringRuleList_Success_Returns200(t *testing.T) {
	svc := &stubScoringRuleSvc{rules: []*domain.ScoringRule{scoringRuleFixtureHandler()}}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRules, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminScoringRuleList_EmptyRules_Returns200(t *testing.T) {
	svc := &stubScoringRuleSvc{rules: []*domain.ScoringRule{}}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRules, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminScoringRuleList_ServiceError_Returns500(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.Internal(nil)}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRules, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── GetByPhase ────────────────────────────────────────────────────────────────

func TestAdminScoringRuleGetByPhase_Success_Returns200(t *testing.T) {
	svc := &stubScoringRuleSvc{rule: scoringRuleFixtureHandler()}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRulesGroupStage, "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminScoringRuleGetByPhase_NotFound_Returns404(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.NotFound("phase not found")}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRulesGroupStage, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminScoringRuleGetByPhase_ServiceError_Returns500(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.Internal(nil)}
	w := do(newScoringRuleRouter(svc), http.MethodGet, pathScoringRulesGroupStage, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestAdminScoringRuleUpdate_Success_Returns200(t *testing.T) {
	svc := &stubScoringRuleSvc{rule: scoringRuleFixtureHandler()}
	body := `{"exact_score":10,"correct_outcome":5,"goal_difference":2,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesGroupStage, body), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminScoringRuleUpdate_NoCallerInContext_Returns401(t *testing.T) {
	svc := &stubScoringRuleSvc{rule: scoringRuleFixtureHandler()}
	body := `{"exact_score":10,"correct_outcome":5,"goal_difference":2,"is_active":true}`
	w := do(newScoringRuleRouter(svc), http.MethodPatch, pathScoringRulesGroupStage, body)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestAdminScoringRuleUpdate_InvalidJSON_Returns400(t *testing.T) {
	svc := &stubScoringRuleSvc{rule: scoringRuleFixtureHandler()}
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesGroupStage, "not-json"), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminScoringRuleUpdate_ValidationError_Returns422(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.Validation("exact_score must be greater than correct_outcome")}
	body := `{"exact_score":2,"correct_outcome":5,"goal_difference":1,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesGroupStage, body), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestAdminScoringRuleUpdate_PhaseNotFound_Returns404(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.NotFound("scoring rule not found for phase")}
	body := `{"exact_score":10,"correct_outcome":5,"goal_difference":2,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesGroupStage, body), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminScoringRuleUpdate_FinalPhase_Success_Returns200(t *testing.T) {
	rule := scoringRuleFixtureHandler()
	rule.Phase = domain.PhaseFinal
	rule.ExactScore = 15
	rule.CorrectOutcome = 8
	rule.GoalDifference = 3
	svc := &stubScoringRuleSvc{rule: rule}
	body := `{"exact_score":15,"correct_outcome":8,"goal_difference":3,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesFinal, body), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestAdminScoringRuleUpdate_ServiceError_Returns500(t *testing.T) {
	svc := &stubScoringRuleSvc{err: apperrors.Internal(nil)}
	body := `{"exact_score":10,"correct_outcome":5,"goal_difference":2,"is_active":true}`
	req := withCaller(newAdminRequestJSON(http.MethodPatch, pathScoringRulesGroupStage, body), adminCaller)
	w := doReq(newScoringRuleRouter(svc), req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
