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
)

// testUserRouter builds a chi router wired with the UserHandler.  When user is
// non-nil it is injected into the request context via the ResolveUser middleware
// shim, matching production behaviour.
func testUserRouter(h *handler.UserHandler, user *domain.User) http.Handler {
	r := chi.NewRouter()
	if user != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(middleware.ContextWithUser(req.Context(), user)))
			})
		})
	}
	r.Get("/users/me", h.GetMe)
	r.Patch("/users/me", h.UpdateMe)
	return r
}

// sampleUser returns a domain.User with all fields populated for assertion.
func sampleUser() *domain.User {
	return &domain.User{
		ID:            42,
		Name:          "Carlos",
		Email:         "carlos@example.com",
		Role:          domain.RoleUser,
		BalanceCents:  10000,
		ReservedCents: 500,
		KYCTier:       domain.KYCTier(1),
		Locale:        "es",
		CreatedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// ── GET /users/me ─────────────────────────────────────────────────────────────

func TestUserHandler_GetMe_200_ReturnsProfile(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d", rr.Code)
	}

	var resp handler.MeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ID != u.ID {
		t.Errorf("ID: got %d; want %d", resp.ID, u.ID)
	}
	if resp.Locale != "es" {
		t.Errorf("Locale: got %q; want \"es\"", resp.Locale)
	}
	if resp.KYCTier != 1 {
		t.Errorf("KYCTier: got %d; want 1", resp.KYCTier)
	}
	if resp.BalanceCents != 10000 {
		t.Errorf("BalanceCents: got %d; want 10000", resp.BalanceCents)
	}
}

func TestUserHandler_GetMe_401_WhenNoUser(t *testing.T) {
	t.Parallel()

	h := handler.NewUserHandler(&stubUserRepo{}, zaptest.NewLogger(t))
	router := testUserRouter(h, nil)

	req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

// ── PATCH /users/me ───────────────────────────────────────────────────────────

func TestUserHandler_UpdateMe_204_LocaleEN(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{"locale":"en"}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, rr.Code)
	}
}

func TestUserHandler_UpdateMe_204_LocaleES(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{"locale":"es"}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, rr.Code)
	}
}

func TestUserHandler_UpdateMe_422_UnsupportedLocale(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{"locale":"fr"}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf(fmtExpect422, rr.Code)
	}

	var resp handler.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Code != "VALIDATION" {
		t.Errorf("error code: got %q; want \"VALIDATION\"", resp.Error.Code)
	}
}

func TestUserHandler_UpdateMe_204_EmptyBody_IsNoop(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf(fmtExpect204, rr.Code)
	}
}

func TestUserHandler_UpdateMe_401_WhenNoUser(t *testing.T) {
	t.Parallel()

	h := handler.NewUserHandler(&stubUserRepo{}, zaptest.NewLogger(t))
	router := testUserRouter(h, nil)

	body := strings.NewReader(`{"locale":"en"}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestUserHandler_UpdateMe_400_MalformedJSON(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{not valid json`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf(fmtExpect400, rr.Code)
	}
}

func TestUserHandler_UpdateMe_500_RepoError(t *testing.T) {
	t.Parallel()

	u := sampleUser()
	h := handler.NewUserHandler(&stubUserRepo{user: u, err: errors.New("db error")}, zaptest.NewLogger(t))
	router := testUserRouter(h, u)

	body := strings.NewReader(`{"locale":"en"}`)
	req := httptest.NewRequest(http.MethodPatch, "/users/me", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}
