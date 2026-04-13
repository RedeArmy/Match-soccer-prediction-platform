// Package middleware_test exercises the HTTP middleware defined in this package.
//
// All tests use net/http/httptest and do not require a real network listener,
// database, or external service. Tests for RequireAuth cover only the cases
// that are testable without a live Clerk JWKS endpoint (missing token,
// malformed header, bypass when JWKS URL is empty). Integration tests against
// a real Clerk token belong in the tests/ directory with a build tag.
package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// stubUserRepo implements repository.UserRepository for middleware tests.
type stubUserRepo struct {
	user *domain.User
	err  error
}

func (r *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) Delete(_ context.Context, _ int) error          { return r.err }
func (r *stubUserRepo) List(_ context.Context) ([]*domain.User, error) { return nil, r.err }

const (
	fmtStatus        = "expected status %d, got %d"
	originLocalhost  = "http://localhost:3000"
	headerACAO       = "Access-Control-Allow-Origin"
	headerAuth       = "Authorization"
	headerOrigin     = "Origin"
	pathMatches      = "/api/v1/matches"
	msgMatchNotFound = "match not found"

	subjectForRole    = "user_abc"
	subjectForResolve = "user_clerk_abc"
)

// okHandler is a trivial handler used as the "next" in middleware chain tests.
// It always writes 200 with a fixed body so tests can assert the chain was
// not short-circuited by the middleware under test.
func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// requestWithID wraps a request in chi's RequestID middleware context so that
// middleware that calls GetRequestID does not receive an empty string.
func requestWithID(r *http.Request) *http.Request {
	rec := httptest.NewRecorder()
	var captured *http.Request
	chimiddleware.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})).ServeHTTP(rec, r)
	return captured
}

// ── GetRequestID ──────────────────────────────────────────────────────────────

func TestGetRequestID_ReturnsIDFromContext(t *testing.T) {
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))

	id := middleware.GetRequestID(req.Context())
	if id == "" {
		t.Error("expected non-empty request ID, got empty string")
	}
}

func TestGetRequestID_ReturnsEmptyWhenNotSet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	id := middleware.GetRequestID(req.Context())
	if id != "" {
		t.Errorf("expected empty string for context without request ID, got %q", id)
	}
}

// ── Recover ───────────────────────────────────────────────────────────────────

func TestRecover_CatchesPanicAndReturns500(t *testing.T) {
	log := zaptest.NewLogger(t)
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	handler := middleware.Recover(log)(panicHandler)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

func TestRecover_DoesNotInterferWithNormalRequests(t *testing.T) {
	log := zaptest.NewLogger(t)
	handler := middleware.Recover(log)(http.HandlerFunc(okHandler))
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtStatus, http.StatusOK, rec.Code)
	}
}

// ── RequestLogger ─────────────────────────────────────────────────────────────

func TestRequestLogger_PassesRequestToNextHandler(t *testing.T) {
	log := zaptest.NewLogger(t)
	handler := middleware.RequestLogger(log)(http.HandlerFunc(okHandler))
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/test", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf(fmtStatus, http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body %q, got %q", "ok", rec.Body.String())
	}
}

func TestRequestLogger_CapturesNonOKStatus(t *testing.T) {
	log := zaptest.NewLogger(t)
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := middleware.RequestLogger(log)(notFoundHandler)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/missing", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf(fmtStatus, http.StatusNotFound, rec.Code)
	}
}

// ── CORS ──────────────────────────────────────────────────────────────────────

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	handler := middleware.CORS([]string{originLocalhost})(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerOrigin, originLocalhost)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get(headerACAO)
	if origin != originLocalhost {
		t.Errorf("expected ACAO header %q, got %q", originLocalhost, origin)
	}
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	handler := middleware.CORS([]string{originLocalhost})(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerOrigin, "http://evil.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get(headerACAO)
	if origin == "http://evil.example.com" {
		t.Error("expected unknown origin to be rejected, but ACAO header was set")
	}
}

func TestCORS_HandlesPreflight(t *testing.T) {
	handler := middleware.CORS([]string{originLocalhost})(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodOptions, pathMatches, nil)
	req.Header.Set(headerOrigin, originLocalhost)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Errorf("expected preflight to return 200 or 204, got %d", rec.Code)
	}
}

func TestCORS_MultipleOriginsAllowed(t *testing.T) {
	handler := middleware.CORS([]string{originLocalhost, "https://myapp.com"})(http.HandlerFunc(okHandler))

	for _, origin := range []string{originLocalhost, "https://myapp.com"} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(headerOrigin, origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			got := rec.Header().Get(headerACAO)
			if got != origin {
				t.Errorf("expected ACAO %q, got %q", origin, got)
			}
		})
	}
}

// ── WriteError ────────────────────────────────────────────────────────────────

func TestWriteError_AppError_WritesCorrectStatusAndBody(t *testing.T) {
	log := zaptest.NewLogger(t)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	middleware.WriteError(rec, req, log, apperrors.NotFound(msgMatchNotFound))

	if rec.Code != http.StatusNotFound {
		t.Errorf(fmtStatus, http.StatusNotFound, rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "NOT_FOUND") {
		t.Errorf("expected body to contain %q, got: %s", "NOT_FOUND", body)
	}
	if !strings.Contains(body, msgMatchNotFound) {
		t.Errorf("expected body to contain %q, got: %s", msgMatchNotFound, body)
	}
}

func TestWriteError_AppError_WithCause_DoesNotLeakCause(t *testing.T) {
	log := zaptest.NewLogger(t)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	middleware.WriteError(rec, req, log, apperrors.Internal(errors.New("pgx: connection refused")))

	body := rec.Body.String()
	if strings.Contains(body, "pgx") {
		t.Errorf("internal cause must not appear in response body, got: %s", body)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

func TestWriteError_UnexpectedError_Returns500(t *testing.T) {
	log := zaptest.NewLogger(t)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	middleware.WriteError(rec, req, log, errors.New("unexpected failure"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

func TestWriteError_SetsJSONContentType(t *testing.T) {
	log := zaptest.NewLogger(t)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	middleware.WriteError(rec, req, log, apperrors.Validation("invalid input"))

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// ── RequireAuth ───────────────────────────────────────────────────────────────

func TestRequireAuth_EmptyJWKSURL_BypassesAuth(t *testing.T) {
	log := zaptest.NewLogger(t)
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.RequireAuth("", log)(next)
	req := httptest.NewRequest(http.MethodGet, pathMatches, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !reached {
		t.Error("expected next handler to be called when JWKS URL is empty")
	}
}

func TestRequireAuth_MissingAuthHeader_Returns401(t *testing.T) {
	log := zap.NewNop()
	handler := middleware.RequireAuth("https://example.clerk.accounts.dev/.well-known/jwks.json", log)(
		http.HandlerFunc(okHandler),
	)
	req := requestWithID(httptest.NewRequest(http.MethodGet, pathMatches, nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuth_NonBearerHeader_Returns401(t *testing.T) {
	log := zap.NewNop()
	handler := middleware.RequireAuth("https://example.clerk.accounts.dev/.well-known/jwks.json", log)(
		http.HandlerFunc(okHandler),
	)
	req := requestWithID(httptest.NewRequest(http.MethodGet, pathMatches, nil))
	req.Header.Set(headerAuth, "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

// TestRequireAuth_JWKSFetchError_Returns500 verifies that a JWKS endpoint that
// returns an error causes RequireAuth to respond 500.
func TestRequireAuth_JWKSFetchError_Returns500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	log := zap.NewNop()
	handler := middleware.RequireAuth(srv.URL, log)(http.HandlerFunc(okHandler))
	req := requestWithID(httptest.NewRequest(http.MethodGet, pathMatches, nil))
	req.Header.Set(headerAuth, "Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyXzEifQ.sig")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

// TestRequireAuth_InvalidToken_Returns401 verifies that a valid JWKS endpoint
// paired with a malformed/unsigned JWT causes RequireAuth to respond 401.
func TestRequireAuth_InvalidToken_Returns401(t *testing.T) {
	// Serve a minimal valid JWKS (empty key set). The token will fail to parse
	// because no matching key exists — this exercises the jwt.Parse error branch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	log := zap.NewNop()
	handler := middleware.RequireAuth(srv.URL, log)(http.HandlerFunc(okHandler))
	req := requestWithID(httptest.NewRequest(http.MethodGet, pathMatches, nil))
	req.Header.Set(headerAuth, "Bearer not.a.jwt")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

// ── UserIDFromContext ─────────────────────────────────────────────────────────

func TestUserIDFromContext_ReturnsFalseWhenNotSet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok := middleware.UserIDFromContext(req.Context())
	if ok {
		t.Error("expected ok=false when user ID not in context, got true")
	}
}

// ── RequireRole ───────────────────────────────────────────────────────────────

func requireRoleHandler(repo *stubUserRepo, roles ...domain.UserRole) http.Handler {
	log := zap.NewNop()
	return middleware.RequireRole(repo, log, roles...)(http.HandlerFunc(okHandler))
}

func requireRoleRequest(subject string) *http.Request {
	req := requestWithID(httptest.NewRequest(http.MethodGet, pathMatches, nil))
	if subject != "" {
		req = req.WithContext(middleware.ContextWithUserID(req.Context(), subject))
	}
	return req
}

func TestRequireRole_NoSubjectInContext_Returns401(t *testing.T) {
	h := requireRoleHandler(&stubUserRepo{}, domain.RoleAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requireRoleRequest(""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireRole_RepoError_Returns500(t *testing.T) {
	repo := &stubUserRepo{err: errors.New("db down")}
	h := requireRoleHandler(repo, domain.RoleAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requireRoleRequest(subjectForRole))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

func TestRequireRole_UserNotFound_Returns401(t *testing.T) {
	repo := &stubUserRepo{user: nil} // subject has no matching row
	h := requireRoleHandler(repo, domain.RoleAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requireRoleRequest(subjectForRole))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireRole_WrongRole_Returns403(t *testing.T) {
	repo := &stubUserRepo{user: &domain.User{ID: 1, Role: domain.RolePlayer}}
	h := requireRoleHandler(repo, domain.RoleAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requireRoleRequest(subjectForRole))
	if rec.Code != http.StatusForbidden {
		t.Errorf(fmtStatus, http.StatusForbidden, rec.Code)
	}
}

func TestRequireRole_CorrectRole_CallsNext(t *testing.T) {
	repo := &stubUserRepo{user: &domain.User{ID: 1, Role: domain.RoleAdmin}}
	h := requireRoleHandler(repo, domain.RoleAdmin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requireRoleRequest(subjectForRole))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtStatus, http.StatusOK, rec.Code)
	}
}

// ── ResolveUser ───────────────────────────────────────────────────────────────

func resolveUserHandler(repo *stubUserRepo) http.Handler {
	log := zap.NewNop()
	return middleware.ResolveUser(repo, log)(http.HandlerFunc(okHandler))
}

// resolveUserRequest builds a request with or without a Clerk subject in context.
func resolveUserRequest(subject string) *http.Request {
	return requireRoleRequest(subject)
}

func TestResolveUser_NoSubjectInContext_Returns401(t *testing.T) {
	h := resolveUserHandler(&stubUserRepo{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, resolveUserRequest(""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

func TestResolveUser_RepoError_Returns500(t *testing.T) {
	repo := &stubUserRepo{err: errors.New("db down")}
	h := resolveUserHandler(repo)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, resolveUserRequest(subjectForResolve))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, rec.Code)
	}
}

func TestResolveUser_UserNotFound_Returns401(t *testing.T) {
	repo := &stubUserRepo{user: nil}
	h := resolveUserHandler(repo)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, resolveUserRequest(subjectForResolve))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, rec.Code)
	}
}

func TestResolveUser_Success_CallsNext(t *testing.T) {
	repo := &stubUserRepo{user: &domain.User{ID: 5, Name: "Alice"}}
	h := resolveUserHandler(repo)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, resolveUserRequest(subjectForResolve))
	if rec.Code != http.StatusOK {
		t.Errorf(fmtStatus, http.StatusOK, rec.Code)
	}
}

// ── UserFromContext / ContextWithUser ─────────────────────────────────────────

func TestContextWithUser_RoundTrip(t *testing.T) {
	want := &domain.User{ID: 42, Name: "Bob"}
	ctx := middleware.ContextWithUser(context.Background(), want)
	got, ok := middleware.UserFromContext(ctx)
	if !ok {
		t.Fatal("UserFromContext returned ok=false after ContextWithUser")
	}
	if got.ID != want.ID {
		t.Errorf("expected user ID %d, got %d", want.ID, got.ID)
	}
}

func TestUserFromContext_ReturnsFalseWhenNotSet(t *testing.T) {
	_, ok := middleware.UserFromContext(context.Background())
	if ok {
		t.Error("expected ok=false when no user in context, got true")
	}
}
