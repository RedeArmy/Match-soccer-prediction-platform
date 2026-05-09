package handler_test

import (
	"encoding/json"
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

const predictionDecodeErrFmt = "decode response: %v"

// newPredRouter wires PredictionHandler into a chi router.
// When withAuth is true, a middleware is prepended that injects the resolved
// domain.User{ID:1} into the request context (simulating ResolveUser middleware).
func newPredRouter(svc *stubPredSvc, withAuth bool) http.Handler {
	r := chi.NewRouter()
	if withAuth {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := middleware.ContextWithUser(req.Context(), &domain.User{ID: 1})
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
	}
	h := handler.NewPredictionHandler(svc, zap.NewNop())
	r.Post("/", h.Submit)
	r.Get("/me", h.GetMine)
	r.Patch("/{id}", h.Update)
	return r
}

func doPred(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(headerContentType, contentTypeJSON)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestSubmit_NoAuthContext_Returns401(t *testing.T) {
	// Request reaches the handler without RequireAuth having set a user ID.
	w := doPred(newPredRouter(&stubPredSvc{}, false), http.MethodPost, "/", bodySubmitPrediction)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestSubmit_InvalidJSON_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, true), http.MethodPost, "/", `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

// Note: "repo error on user lookup" and "user not synced" cases are now covered
// by ResolveUser middleware tests - they are not reachable from the handler
// since user resolution happens before the handler is called.

func TestSubmit_ServiceError_Returns422(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Validation("past deadline")}
	w := doPred(newPredRouter(svc, true), http.MethodPost, "/", bodySubmitPrediction)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestSubmit_Success_Returns201(t *testing.T) {
	svc := &stubPredSvc{created: true, pred: &domain.Prediction{ID: 1, UserID: 1, MatchID: 1}}
	w := doPred(newPredRouter(svc, true), http.MethodPost, "/", bodySubmitPrediction)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestSubmit_IdempotentReplay_Returns200(t *testing.T) {
	svc := &stubPredSvc{created: false, pred: &domain.Prediction{ID: 1, UserID: 1, MatchID: 1}}
	w := doPred(newPredRouter(svc, true), http.MethodPost, "/", bodySubmitPrediction)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on idempotent replay, got %d", w.Code)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestPredUpdate_Success_Returns200(t *testing.T) {
	svc := &stubPredSvc{pred: &domain.Prediction{ID: 1, HomeScore: 2, AwayScore: 1}}
	w := doPred(newPredRouter(svc, true), http.MethodPatch, pathPredictionID1, bodyUpdatePrediction)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	if svc.updateCallerID != 1 || svc.updateID != 1 {
		t.Errorf("expected caller/user IDs to be propagated, got caller=%d id=%d", svc.updateCallerID, svc.updateID)
	}
}

func TestPredUpdate_NoAuthContext_Returns401(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, false), http.MethodPatch, pathPredictionID1, bodyUpdatePrediction)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestPredUpdate_InvalidID_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, true), http.MethodPatch, "/abc", bodyUpdatePrediction)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestPredUpdate_InvalidJSON_Returns422(t *testing.T) {
	w := doPred(newPredRouter(&stubPredSvc{}, true), http.MethodPatch, pathPredictionID1, `not json`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestPredUpdate_ServiceError_Returns404(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.NotFound("prediction not found")}
	w := doPred(newPredRouter(svc, true), http.MethodPatch, pathPredictionID1, bodyUpdatePrediction)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPredUpdate_AnotherUsersPrediction_Returns403(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Forbidden("cannot modify another user's prediction")}
	w := doPred(newPredRouter(svc, true), http.MethodPatch, pathPredictionID1, bodyUpdatePrediction)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ── GetMine ───────────────────────────────────────────────────────────────────

func TestGetMine_NoAuthContext_Returns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, urlGetMyPredictions, nil)
	w := httptest.NewRecorder()
	newPredRouter(&stubPredSvc{}, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, w.Code)
	}
}

func TestGetMine_Success_Returns200(t *testing.T) {
	svc := &stubPredSvc{preds: []*domain.Prediction{{ID: 1, UserID: 1}}}
	req := httptest.NewRequest(http.MethodGet, urlGetMyPredictions, nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var got handler.Paged[handler.PredictionResponse]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf(predictionDecodeErrFmt, err)
	}
	if len(got.Data) != 1 || got.Data[0].UserID != 1 {
		t.Errorf("expected 1 prediction for caller, got %+v", got.Data)
	}
}

func TestGetMine_ServiceError_Returns500(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Internal(nil)}
	req := httptest.NewRequest(http.MethodGet, urlGetMyPredictions, nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetMine with quiniela_id filter ──────────────────────────────────────────

func TestGetMine_WithQuinielaID_Success_Returns200(t *testing.T) {
	svc := &stubPredSvc{preds: []*domain.Prediction{{ID: 1, UserID: 1, MatchID: 5}}}
	req := httptest.NewRequest(http.MethodGet, "/me?quiniela_id=3", nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var got handler.Paged[handler.PredictionResponse]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf(predictionDecodeErrFmt, err)
	}
	if len(got.Data) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(got.Data))
	}
}

func TestGetMine_WithQuinielaID_InvalidParam_Returns422(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/me?quiniela_id=abc", nil)
	w := httptest.NewRecorder()
	newPredRouter(&stubPredSvc{}, true).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, w.Code)
	}
}

func TestGetMine_WithQuinielaID_NonMember_ReturnsEmpty200(t *testing.T) {
	svc := &stubPredSvc{preds: []*domain.Prediction{}}
	req := httptest.NewRequest(http.MethodGet, "/me?quiniela_id=99", nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var got handler.Paged[handler.PredictionResponse]
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf(predictionDecodeErrFmt, err)
	}
	if len(got.Data) != 0 {
		t.Errorf("expected empty array for non-member, got %d entries", len(got.Data))
	}
}

func TestGetMine_WithQuinielaID_ServiceError_Returns500(t *testing.T) {
	svc := &stubPredSvc{err: apperrors.Internal(nil)}
	req := httptest.NewRequest(http.MethodGet, "/me?quiniela_id=3", nil)
	w := httptest.NewRecorder()
	newPredRouter(svc, true).ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}
