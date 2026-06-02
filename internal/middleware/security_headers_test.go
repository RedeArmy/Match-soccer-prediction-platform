package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

func TestSecurityHeaders_PresentOn200(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	assertHeader(t, rr, "Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	assertHeader(t, rr, "X-Content-Type-Options", "nosniff")
	assertHeader(t, rr, "X-Frame-Options", "DENY")
	assertHeader(t, rr, "Referrer-Policy", "strict-origin-when-cross-origin")
	assertHeader(t, rr, "Content-Security-Policy", "default-src 'none'")
}

func TestSecurityHeaders_PresentOn401(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil))
	assertHeader(t, rr, "Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	assertHeader(t, rr, "X-Content-Type-Options", "nosniff")
	assertHeader(t, rr, "X-Frame-Options", "DENY")
}

func TestSecurityHeaders_PresentOn404(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/not-found", nil))
	assertHeader(t, rr, "Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	assertHeader(t, rr, "X-Content-Type-Options", "nosniff")
	assertHeader(t, rr, "X-Frame-Options", "DENY")
}

// TestSecurityHeaders_HSTS_MaxAgeIsOneYear verifies the exact HSTS directive
// so that a value change (e.g. accidentally dropping includeSubDomains or
// reducing max-age) is caught immediately rather than going unnoticed.
func TestSecurityHeaders_HSTS_DirectivesAreCorrect(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	const wantHSTS = "max-age=31536000; includeSubDomains"
	got := rr.Header().Get("Strict-Transport-Security")
	if got != wantHSTS {
		t.Errorf("Strict-Transport-Security: got %q, want %q", got, wantHSTS)
	}
}

func assertHeader(t *testing.T, rr *httptest.ResponseRecorder, key, want string) {
	t.Helper()
	if got := rr.Header().Get(key); got != want {
		t.Errorf("header %q: got %q, want %q", key, got, want)
	}
}
