// Package middleware_test exercises the HTTP middleware defined in this package.
//
// All tests use net/http/httptest and do not require a real network listener,
// database, or external service. Tests for RequireAuth cover only the cases
// that are testable without a live Clerk JWKS endpoint (missing token,
// malformed header, bypass when JWKS URL is empty). Integration tests against
// a real Clerk token belong in the tests/ directory with a build tag.
package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
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
		t.Errorf("expected status %d after panic, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRecover_DoesNotInterferWithNormalRequests(t *testing.T) {
	log := zaptest.NewLogger(t)
	handler := middleware.Recover(log)(http.HandlerFunc(okHandler))
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d for normal request, got %d", http.StatusOK, rec.Code)
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
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
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
		t.Errorf("expected status %d to pass through, got %d", http.StatusNotFound, rec.Code)
	}
}

// ── CORS ──────────────────────────────────────────────────────────────────────

func TestCORS_AllowsConfiguredOrigin(t *testing.T) {
	handler := middleware.CORS("http://localhost:3000")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("expected ACAO header %q, got %q", "http://localhost:3000", origin)
	}
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	handler := middleware.CORS("http://localhost:3000")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin == "http://evil.example.com" {
		t.Error("expected unknown origin to be rejected, but ACAO header was set")
	}
}

func TestCORS_HandlesPreflight(t *testing.T) {
	handler := middleware.CORS("http://localhost:3000")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/matches", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Errorf("expected preflight to return 200 or 204, got %d", rec.Code)
	}
}

func TestCORS_MultipleOriginsAllowed(t *testing.T) {
	handler := middleware.CORS("http://localhost:3000,https://myapp.com")(http.HandlerFunc(okHandler))

	for _, origin := range []string{"http://localhost:3000", "https://myapp.com"} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Origin", origin)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Access-Control-Allow-Origin")
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

	middleware.WriteError(rec, req, log, apperrors.NotFound("match not found"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "NOT_FOUND") {
		t.Errorf("expected body to contain %q, got: %s", "NOT_FOUND", body)
	}
	if !strings.Contains(body, "match not found") {
		t.Errorf("expected body to contain %q, got: %s", "match not found", body)
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
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestWriteError_UnexpectedError_Returns500(t *testing.T) {
	log := zaptest.NewLogger(t)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/", nil))
	rec := httptest.NewRecorder()

	middleware.WriteError(rec, req, log, errors.New("unexpected failure"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/matches", nil)
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
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/api/v1/matches", nil))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for missing auth header, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuth_NonBearerHeader_Returns401(t *testing.T) {
	log := zap.NewNop()
	handler := middleware.RequireAuth("https://example.clerk.accounts.dev/.well-known/jwks.json", log)(
		http.HandlerFunc(okHandler),
	)
	req := requestWithID(httptest.NewRequest(http.MethodGet, "/api/v1/matches", nil))
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for non-Bearer token, got %d", http.StatusUnauthorized, rec.Code)
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
