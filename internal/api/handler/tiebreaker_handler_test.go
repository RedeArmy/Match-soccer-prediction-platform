package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	pathTiebreakerBase            = "/groups/1/tiebreaker"
	pathTiebreakerQuestion        = "/tiebreaker/question"
	pathTiebreakerResult          = "/tiebreaker/result"
	tiebreakerHandlerDBError      = "db error"
	tiebreakerHandlerQuestion     = "Total goals"
	tiebreakerHandler4xxFmt       = "expected 4xx, got %d"
	tiebreakerHandlerBodyQuestion = "{\"question\":\"" + tiebreakerHandlerQuestion + "\"}"
)

// testTiebreakerRouter mounts admin routes at /tiebreaker and member routes
// under /groups/{id}/tiebreaker, mirroring the production server layout.
func testTiebreakerRouter(h *handler.TiebreakerHandler, user *domain.User) http.Handler {
	r := chi.NewRouter()
	if user != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(middleware.ContextWithUser(req.Context(), user)))
			})
		})
	}
	// Admin routes — no group ID required.
	r.Patch("/tiebreaker/question", h.SetQuestion)
	r.Patch("/tiebreaker/result", h.ConfirmResult)
	// Member routes — scoped to a specific group.
	r.Route("/groups/{id}", func(r chi.Router) {
		r.Post("/tiebreaker", h.Submit)
		r.Get("/tiebreaker", h.GetMine)
	})
	return r
}

func tiebreakerHandler(svc *stubTiebreakerSvc, t *testing.T) *handler.TiebreakerHandler {
	return handler.NewTiebreakerHandler(svc, zaptest.NewLogger(t))
}

// ── SetQuestion ───────────────────────────────────────────────────────────────

func TestTiebreakerHandler_SetQuestion_401_WhenNoUser(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{config: &domain.TiebreakerConfig{}}, t)
	router := testTiebreakerRouter(h, nil)

	body := strings.NewReader(tiebreakerHandlerBodyQuestion)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerQuestion, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTiebreakerHandler_SetQuestion_500_WhenServiceFails(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: errors.New(tiebreakerHandlerDBError)}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(tiebreakerHandlerBodyQuestion)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerQuestion, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

func TestTiebreakerHandler_SetQuestion_403_WhenForbidden(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: apperrors.Forbidden("not admin")}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 99})

	body := strings.NewReader(tiebreakerHandlerBodyQuestion)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerQuestion, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestTiebreakerHandler_SetQuestion_200_ReturnsConfig(t *testing.T) {
	question := tiebreakerHandlerQuestion
	cfg := &domain.TiebreakerConfig{
		ID:        1,
		Question:  question,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	h := tiebreakerHandler(&stubTiebreakerSvc{config: cfg}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(tiebreakerHandlerBodyQuestion)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerQuestion, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	if ct := rr.Header().Get(headerContentType); !strings.HasPrefix(ct, contentTypeJSON) {
		t.Errorf("Content-Type: want %s, got %s", contentTypeJSON, ct)
	}

	var resp handler.TiebreakerConfigResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Question != question {
		t.Errorf("Question: want %q, got %q", question, resp.Question)
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestTiebreakerHandler_Submit_401_WhenNoUser(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{tb: &domain.Tiebreaker{}}, t)
	router := testTiebreakerRouter(h, nil)

	body := strings.NewReader(`{"prediction":5}`)
	req := httptest.NewRequest(http.MethodPost, pathTiebreakerBase, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTiebreakerHandler_Submit_500_WhenServiceFails(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: errors.New(tiebreakerHandlerDBError)}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	body := strings.NewReader(`{"prediction":5}`)
	req := httptest.NewRequest(http.MethodPost, pathTiebreakerBase, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

func TestTiebreakerHandler_Submit_200_ReturnsTiebreaker(t *testing.T) {
	tb := &domain.Tiebreaker{
		ID:         3,
		UserID:     42,
		Prediction: 5,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	h := tiebreakerHandler(&stubTiebreakerSvc{tb: tb}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	body := strings.NewReader(`{"prediction":5}`)
	req := httptest.NewRequest(http.MethodPost, pathTiebreakerBase, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}

	var resp handler.TiebreakerResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Prediction != 5 {
		t.Errorf("Prediction: want 5, got %d", resp.Prediction)
	}
}

// ── GetMine ───────────────────────────────────────────────────────────────────

func TestTiebreakerHandler_GetMine_401_WhenNoUser(t *testing.T) {
	question := "Total goals"
	h := tiebreakerHandler(&stubTiebreakerSvc{view: &domain.TiebreakerView{Question: &question}}, t)
	router := testTiebreakerRouter(h, nil)

	req := httptest.NewRequest(http.MethodGet, pathTiebreakerBase, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTiebreakerHandler_GetMine_500_WhenServiceFails(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: errors.New(tiebreakerHandlerDBError)}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodGet, pathTiebreakerBase, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

func TestTiebreakerHandler_GetMine_200_ReturnsView(t *testing.T) {
	question := tiebreakerHandlerQuestion
	tb := &domain.Tiebreaker{
		ID:         3,
		UserID:     42,
		Prediction: 8,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	view := &domain.TiebreakerView{Question: &question, Entry: tb}
	h := tiebreakerHandler(&stubTiebreakerSvc{view: view}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodGet, pathTiebreakerBase, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}

	var resp handler.TiebreakerViewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Question == nil || *resp.Question != question {
		t.Errorf("Question: want %q, got %v", question, resp.Question)
	}
	if resp.Entry == nil || resp.Entry.Prediction != 8 {
		t.Errorf("Entry.Prediction: want 8, got %v", resp.Entry)
	}
}

func TestTiebreakerHandler_GetMine_200_EntryNilWhenNotSubmitted(t *testing.T) {
	question := tiebreakerHandlerQuestion
	view := &domain.TiebreakerView{Question: &question, Entry: nil}
	h := tiebreakerHandler(&stubTiebreakerSvc{view: view}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodGet, pathTiebreakerBase, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}

	var resp handler.TiebreakerViewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Entry != nil {
		t.Error("Entry: want nil when member has not submitted")
	}
}

// ── ConfirmResult ─────────────────────────────────────────────────────────────

func TestTiebreakerHandler_ConfirmResult_401_WhenNoUser(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{}, t)
	router := testTiebreakerRouter(h, nil)

	body := strings.NewReader(`{"result":10}`)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerResult, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestTiebreakerHandler_ConfirmResult_403_WhenForbidden(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: apperrors.Forbidden("not admin")}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 99})

	body := strings.NewReader(`{"result":10}`)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerResult, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestTiebreakerHandler_ConfirmResult_204_WhenSucceeds(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"result":10}`)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerResult, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, rr.Code)
	}
}

func TestTiebreakerHandler_ConfirmResult_500_WhenServiceFails(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{err: errors.New(tiebreakerHandlerDBError)}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	body := strings.NewReader(`{"result":10}`)
	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerResult, body)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

// ── Bad request (malformed JSON body) ────────────────────────────────────────

func TestTiebreakerHandler_SetQuestion_400_WhenBadBody(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{config: &domain.TiebreakerConfig{}}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerQuestion, strings.NewReader(`{bad json}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(tiebreakerHandler4xxFmt, rr.Code)
	}
}

func TestTiebreakerHandler_Submit_400_WhenBadBody(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{tb: &domain.Tiebreaker{}}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodPost, pathTiebreakerBase, strings.NewReader(`{bad json}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(tiebreakerHandler4xxFmt, rr.Code)
	}
}

func TestTiebreakerHandler_ConfirmResult_400_WhenBadBody(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodPatch, pathTiebreakerResult, strings.NewReader(`{bad json}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf(tiebreakerHandler4xxFmt, rr.Code)
	}
}

// ── Invalid group ID (member routes only) ────────────────────────────────────

func TestTiebreakerHandler_Submit_422_WhenInvalidID(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodPost, "/groups/abc/tiebreaker", strings.NewReader(`{"prediction":5}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf("expected 4xx for invalid group ID, got %d", rr.Code)
	}
}

func TestTiebreakerHandler_GetMine_422_WhenInvalidID(t *testing.T) {
	h := tiebreakerHandler(&stubTiebreakerSvc{}, t)
	router := testTiebreakerRouter(h, &domain.User{ID: 42})

	req := httptest.NewRequest(http.MethodGet, "/groups/abc/tiebreaker", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code < 400 {
		t.Errorf("expected 4xx for invalid group ID, got %d", rr.Code)
	}
}
