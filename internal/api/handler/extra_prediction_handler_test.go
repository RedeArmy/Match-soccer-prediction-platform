package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const bodySubmitExtra = `{"match_id":1,"extra_type":"first_scorer","answer":"home"}`

// newExtraPredRouter wires ExtraPredictionHandler into a chi router.
func newExtraPredRouter(svc *stubExtraPredSvc, withAuth bool) http.Handler {
	r := chi.NewRouter()
	if withAuth {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 1})
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
	}
	h := handler.NewExtraPredictionHandler(svc, zap.NewNop())
	r.Post("/", h.Submit)
	r.Get("/me", h.GetMine)
	return r
}

func doExtra(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(headerContentType, contentTypeJSON)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestExtraSubmitHandler_NoAuthContext_Returns401(t *testing.T) {
	w := doExtra(newExtraPredRouter(&stubExtraPredSvc{}, false), http.MethodPost, "/", bodySubmitExtra)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestExtraSubmitHandler_InvalidJSON_Returns422(t *testing.T) {
	w := doExtra(newExtraPredRouter(&stubExtraPredSvc{}, true), http.MethodPost, "/", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestExtraSubmitHandler_ServiceError_Returns422(t *testing.T) {
	svc := &stubExtraPredSvc{err: apperrors.Validation("past deadline")}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodPost, "/", bodySubmitExtra)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestExtraSubmitHandler_MatchNotFound_Returns404(t *testing.T) {
	svc := &stubExtraPredSvc{err: apperrors.NotFound("match not found")}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodPost, "/", bodySubmitExtra)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestExtraSubmitHandler_Success_Returns200(t *testing.T) {
	svc := &stubExtraPredSvc{pred: &domain.ExtraPrediction{ID: 1, UserID: 1, MatchID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"}}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodPost, "/", bodySubmitExtra)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

// ── GetMine ───────────────────────────────────────────────────────────────────

func TestExtraGetMineHandler_NoAuthContext_Returns401(t *testing.T) {
	w := doExtra(newExtraPredRouter(&stubExtraPredSvc{}, false), http.MethodGet, "/me?match_ids=1,2", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestExtraGetMineHandler_Success_Returns200(t *testing.T) {
	svc := &stubExtraPredSvc{preds: []*domain.ExtraPrediction{
		{ID: 1, UserID: 1, MatchID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodGet, "/me?match_ids=1,2,3", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestExtraGetMineHandler_EmptyMatchIDs_Returns200(t *testing.T) {
	svc := &stubExtraPredSvc{preds: []*domain.ExtraPrediction{}}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodGet, "/me", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestExtraGetMineHandler_InvalidMatchIDs_Returns422(t *testing.T) {
	w := doExtra(newExtraPredRouter(&stubExtraPredSvc{}, true), http.MethodGet, "/me?match_ids=abc,def", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestExtraGetMineHandler_NegativeMatchID_Returns422(t *testing.T) {
	w := doExtra(newExtraPredRouter(&stubExtraPredSvc{}, true), http.MethodGet, "/me?match_ids=1,-2", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestExtraGetMineHandler_ServiceError_Returns500(t *testing.T) {
	svc := &stubExtraPredSvc{err: apperrors.Internal(nil)}
	w := doExtra(newExtraPredRouter(svc, true), http.MethodGet, "/me?match_ids=1", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
