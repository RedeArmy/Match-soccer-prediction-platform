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
	assertHeader(t, rr, "X-Content-Type-Options", "nosniff")
	assertHeader(t, rr, "X-Frame-Options", "DENY")
}

func TestSecurityHeaders_PresentOn404(t *testing.T) {
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/not-found", nil))
	assertHeader(t, rr, "X-Content-Type-Options", "nosniff")
	assertHeader(t, rr, "X-Frame-Options", "DENY")
}

func assertHeader(t *testing.T, rr *httptest.ResponseRecorder, key, want string) {
	t.Helper()
	if got := rr.Header().Get(key); got != want {
		t.Errorf("header %q: got %q, want %q", key, got, want)
	}
}
