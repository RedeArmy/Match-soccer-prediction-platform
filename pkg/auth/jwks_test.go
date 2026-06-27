package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/pkg/auth"
)

// ── fail-closed provider (empty URL) ─────────────────────────────────────────

func TestNewJWKSProvider_EmptyURL_AlwaysReturnsProviderUnavailable(t *testing.T) {
	p := auth.NewJWKSProvider(context.Background(), "", auth.DefaultWarmupTimeout, zap.NewNop())

	_, err := p.ValidateToken(context.Background(), "any.token.here")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("empty-URL provider must return ErrProviderUnavailable, got %v", err)
	}
}

// ── JWKS endpoint errors ──────────────────────────────────────────────────────

// TestJWKSProvider_FetchError_NoFallback_ReturnsProviderUnavailable verifies
// that when the JWKS endpoint is unreachable and no cached keyset is available
// (warmup also failed), ValidateToken returns ErrProviderUnavailable.
func TestJWKSProvider_FetchError_NoFallback_ReturnsProviderUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	_, err := p.ValidateToken(context.Background(), "some.token.value")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable when JWKS is unreachable, got %v", err)
	}
}

// ── invalid token ─────────────────────────────────────────────────────────────

// TestJWKSProvider_InvalidToken_ReturnsInvalidToken verifies that when the
// JWKS endpoint is healthy but the JWT is malformed or cannot be validated,
// ValidateToken returns ErrInvalidToken (not ErrProviderUnavailable).
func TestJWKSProvider_InvalidToken_ReturnsInvalidToken(t *testing.T) {
	// Serve a valid JWKS with an empty key set. Any JWT will fail to parse
	// (no matching key), exercising the token-validation error branch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	_, err := p.ValidateToken(context.Background(), "not.a.jwt")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for malformed JWT, got %v", err)
	}
}

// TestJWKSProvider_ErrorWrapping_PreservesIs verifies that the sentinel errors
// remain detectable via errors.Is even when wrapped with additional context.
func TestJWKSProvider_ErrorWrapping_PreservesIs(t *testing.T) {
	p := auth.NewJWKSProvider(context.Background(), "", auth.DefaultWarmupTimeout, zap.NewNop())

	_, err := p.ValidateToken(context.Background(), "ignored")
	if err == nil {
		t.Fatal("expected non-nil error from fail-closed provider")
	}
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("errors.Is(err, ErrProviderUnavailable) must be true even when wrapped; err = %v", err)
	}
}

// TestJWKSProvider_WithOnDegraded_NotCalledOnHealthyEndpoint verifies that the
// WithOnDegraded hook is NOT invoked when the JWKS endpoint is reachable and the
// jwk.Cache serves a warm keyset. The hook must only fire on the fallback path.
func TestJWKSProvider_WithOnDegraded_NotCalledOnHealthyEndpoint(t *testing.T) {
	validJWKS := `{"keys":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte(validJWKS))
	}))
	defer srv.Close()

	var hookCalls atomic.Int64
	p := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithOnDegraded(func(_ context.Context) { hookCalls.Add(1) }),
	)

	// Token validation fails (no keys to verify against) but the JWKS endpoint
	// was healthy — the degraded hook must NOT have been invoked.
	_, err := p.ValidateToken(context.Background(), "not.a.real.jwt")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for unverifiable JWT, got %v", err)
	}
	if hookCalls.Load() != 0 {
		t.Errorf("WithOnDegraded hook must not fire when JWKS endpoint is healthy; fired %d time(s)", hookCalls.Load())
	}
}
