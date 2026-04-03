// Package api_test exercises the HTTP surface of the API server using
// httptest rather than a real network listener.
//
// Tests in this file are black-box: they import the api package as an
// external consumer (package api_test) and interact only through the
// public API (api.New, *Server.Routes). This mirrors how the application
// is used in production and catches integration issues that unit tests of
// individual handlers would miss — for example, a middleware that intercepts
// all requests and short-circuits a handler unexpectedly.
//
// The database pool is passed as nil in all tests below. Handlers that
// require a live database are expected to return 503 when db is nil,
// not panic. This contract is verified separately per handler; the tests
// here focus solely on the server's routing and infrastructure endpoints.
package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"go.uber.org/zap/zaptest"
)

// newTestServer constructs a Server with a nil database pool and a
// test-scoped logger that writes to t.Log. It is intended for use in
// tests that exercise routing and infrastructure endpoints only.
func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	return api.New(nil, &config.Config{}, zaptest.NewLogger(t))
}

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHealthEndpoint_ReturnsJSONContentType(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type %q, got %q", "application/json", ct)
	}
}

func TestHealthEndpoint_BodyContainsStatusOK(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected body to contain %q, got: %s", `"status":"ok"`, body)
	}
	if !strings.Contains(body, `"service":"world-cup-quiniela"`) {
		t.Errorf("expected body to contain service name, got: %s", body)
	}
}

func TestHealthEndpoint_OnlyAcceptsGET(t *testing.T) {
	handler := newTestServer(t).Routes()

	// chi returns 405 for registered paths with wrong method, and
	// 404 for unregistered paths. /health is registered for GET only.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected status %d, got %d",
				method, http.StatusMethodNotAllowed, rec.Code)
		}
	}
}

func TestUnknownRoute_Returns404(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}
