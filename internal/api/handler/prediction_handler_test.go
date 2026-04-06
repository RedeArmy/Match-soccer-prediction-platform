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

// newPredRouter wires PredictionHandler into a chi router.
// When withAuth is true, a middleware is prepended that injects userID "1"
// (a valid numeric Clerk subject) into the request context.
func newPredRouter(svc *stubPredSvc, withAuth bool) http.Handler {
	r := chi.NewRouter()
	if withAuth {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(middleware.ContextWithUserID(req.Context(), "1")))
			})
		})
	}
	h := handler.NewPredictionHandler(svc, zap.NewNop())
	r.Post("/", h.Submit)
	r.Get("/", h.ListByUser)
	r.Patch("/{id}", h.Update)
	return r
}

func doPred(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestSubmit_NoAuthContext_Returns401(t *testing.T) {
	// Request reaches the handler without RequireAuth having set a user ID.
	w := doPred(newPredRouter(&stubPredSvc{}, false), http.MethodPost, "/",
		`{"match_id":1,"home_score":2,"away_score":1}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubmit_InvalidJSON_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, true), http.MethodPost, "/", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestSubmit_InvalidUserID_Returns401(t *testing.T) {
	// Inject a non-numeric Clerk subject — clerkSubjectToUserID will fail.
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(middleware.ContextWithUserID(req.Context(), "user_abc")))
		})
	})
	h := handler.NewPredictionHandler(&stubPredSvc{}, zap.NewNop())
	r.Post("/", h.Submit)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"match_id":1,"home_score":1,"away_score":0}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubmit_ServiceError_Returns422(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Validation("past deadline")}
	w := doPred(newPredRouter(svc, true), http.MethodPost, "/",
		`{"match_id":1,"home_score":2,"away_score":1}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestSubmit_Success_Returns201(t *testing.T) {
	svc := &stubPredSvc{pred: &domain.Prediction{ID: 1, UserID: 1, MatchID: 1}}
	w := doPred(newPredRouter(svc, true), http.MethodPost, "/",
		`{"match_id":1,"home_score":2,"away_score":1}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestPredUpdate_Success_Returns200(t *testing.T) {
	svc := &stubPredSvc{pred: &domain.Prediction{ID: 1, HomeScore: 2, AwayScore: 1}}
	w := doPred(newPredRouter(svc, false), http.MethodPatch, "/1", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestPredUpdate_InvalidID_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, false), http.MethodPatch, "/abc", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestPredUpdate_InvalidJSON_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, false), http.MethodPatch, "/1", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestPredUpdate_ServiceError_Returns404(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.NotFound("prediction not found")}
	w := doPred(newPredRouter(svc, false), http.MethodPatch, "/1", `{"home_score":2,"away_score":1}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── ListByUser ────────────────────────────────────────────────────────────────

func TestListByUser_Success_Returns200(t *testing.T) {
	svc := &stubPredSvc{preds: []*domain.Prediction{{ID: 1, UserID: 1}}}
	req := httptest.NewRequest(http.MethodGet, urlListByUserID1, nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, false).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}

func TestListByUser_MissingUserID_Returns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	newPredRouter(&stubPredSvc{}, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestListByUser_InvalidUserID_Returns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?user_id=abc", nil)
	w := httptest.NewRecorder()
	newPredRouter(&stubPredSvc{}, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestListByUser_ServiceError_Returns500(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Internal(nil)}
	req := httptest.NewRequest(http.MethodGet, urlListByUserID1, nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, false).ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// TestListByUser_AuthContextMismatch_Returns403 verifies that an authenticated
// caller cannot retrieve another user's predictions. The auth middleware injects
// userID "1" into context; the request asks for user_id=2.
func TestListByUser_AuthContextMismatch_Returns403(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?user_id=2", nil)
	w := httptest.NewRecorder()
	newPredRouter(&stubPredSvc{}, true).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestListByUser_AuthContextMatch_Returns200 verifies that the authenticated
// caller can retrieve their own predictions when user_id matches the token.
func TestListByUser_AuthContextMatch_Returns200(t *testing.T) {
	svc := &stubPredSvc{preds: []*domain.Prediction{{ID: 1, UserID: 1}}}
	req := httptest.NewRequest(http.MethodGet, urlListByUserID1, nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}
