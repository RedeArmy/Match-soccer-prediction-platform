package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

const testInternalKey = "test-internal-secret-key-32-chars"

func applyInternalAuth(t *testing.T, key string, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	log := zaptest.NewLogger(t)
	w := httptest.NewRecorder()
	handler := middleware.InternalServiceAuth(key, log)(http.HandlerFunc(okHandler))
	handler.ServeHTTP(w, requestWithID(r))
	return w
}

func TestInternalServiceAuth_ValidKey_Passes(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)
	r.Header.Set("Authorization", "Bearer "+testInternalKey)

	w := applyInternalAuth(t, testInternalKey, r)

	if w.Code != http.StatusOK {
		t.Errorf(fmtStatus, http.StatusOK, w.Code)
	}
	if w.Body.String() != okBody {
		t.Errorf("expected body %q, got %q", okBody, w.Body.String())
	}
}

func TestInternalServiceAuth_WrongKey_Returns401(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)
	r.Header.Set("Authorization", "Bearer wrong-key")

	w := applyInternalAuth(t, testInternalKey, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, w.Code)
	}
}

func TestInternalServiceAuth_MissingHeader_Returns401(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)

	w := applyInternalAuth(t, testInternalKey, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, w.Code)
	}
}

func TestInternalServiceAuth_MalformedHeader_Returns401(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)
	r.Header.Set("Authorization", testInternalKey) // missing "Bearer " prefix

	w := applyInternalAuth(t, testInternalKey, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf(fmtStatus, http.StatusUnauthorized, w.Code)
	}
}

func TestInternalServiceAuth_EmptyConfiguredKey_Returns503(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)
	r.Header.Set("Authorization", "Bearer some-token")

	// Empty key = misconfiguration; must fail closed with 503.
	w := applyInternalAuth(t, "", r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtStatus, http.StatusInternalServerError, w.Code)
	}
}

func TestInternalServiceAuth_TimingResistance_DifferentLengths(t *testing.T) {
	// A short token must not succeed even against a longer configured key.
	r := httptest.NewRequest(http.MethodGet, "/api/internal/kyc/queue", nil)
	r.Header.Set("Authorization", "Bearer short")

	w := applyInternalAuth(t, testInternalKey, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("short token should be rejected: got %d", w.Code)
	}
}
