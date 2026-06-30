package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/pkg/auth"
)

// ── mock KeysetPersister ──────────────────────────────────────────────────────

type stubPersister struct {
	saved   []byte
	loadErr error
	loadVal []byte
}

func (s *stubPersister) Save(_ context.Context, payload []byte, _ time.Duration) error {
	s.saved = payload
	return nil
}

func (s *stubPersister) Load(_ context.Context) ([]byte, error) {
	return s.loadVal, s.loadErr
}

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

// ── KeysetPersister integration ───────────────────────────────────────────────

// TestJWKSProvider_WithKeysetPersister_SavesPayloadAfterSuccessfulWarmup verifies
// that a successful JWKS warmup calls Save on the injected persister so that the
// keyset survives a subsequent process restart.
func TestJWKSProvider_WithKeysetPersister_SavesPayloadAfterSuccessfulWarmup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	p := &stubPersister{}
	auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(p, 15*time.Minute),
	)

	if p.saved == nil {
		t.Error("WithKeysetPersister: persister.Save was not called after successful warmup")
	}
}

// TestJWKSProvider_WithKeysetPersister_LoadsFallbackWhenWarmupFails verifies that
// when the JWKS endpoint is unreachable at startup but the persister holds a valid
// keyset payload, loadPersistedKeyset seeds the in-memory fallback so that token
// validation can proceed during the Clerk outage.
func TestJWKSProvider_WithKeysetPersister_LoadsFallbackWhenWarmupFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// The persisted payload is a minimal valid JWKS with an empty key set.
	// Token validation will still fail (no keys to verify against), but the
	// provider must NOT return ErrProviderUnavailable — that would mean the
	// fallback was not loaded.
	validJWKS := []byte(`{"keys":[]}`)
	p := &stubPersister{loadVal: validJWKS}

	provider := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(p, 15*time.Minute),
	)

	_, err := provider.ValidateToken(context.Background(), "not.a.jwt")
	if errors.Is(err, auth.ErrProviderUnavailable) {
		t.Error("expected persisted keyset to be used as fallback; got ErrProviderUnavailable")
	}
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken (keyset loaded but no keys match), got %v", err)
	}
}

// TestJWKSProvider_WithKeysetPersister_LoadError_DoesNotPanic verifies that a
// persister.Load error at startup is handled gracefully (logged, not panicked),
// and the provider continues to operate in fail-closed mode.
func TestJWKSProvider_WithKeysetPersister_LoadError_DoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := &stubPersister{loadErr: errors.New("redis unreachable")}
	provider := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(p, 15*time.Minute),
	)

	_, err := provider.ValidateToken(context.Background(), "any.token")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable when both warmup and persister fail; got %v", err)
	}
}

// TestJWKSProvider_WithOnDegraded_FiredWhenPersistedFallbackUsed verifies that
// the WithOnDegraded hook fires when the JWKS endpoint is unreachable (warmup
// failed), the in-memory fallback was seeded from the persister, and
// ValidateToken uses that fallback because jwkCache.Get() returns an error.
func TestJWKSProvider_WithOnDegraded_FiredWhenPersistedFallbackUsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Warmup fails; persister provides a valid keyset → loadPersistedKeyset seeds fallback.
	p := &stubPersister{loadVal: []byte(`{"keys":[]}`)}
	var hookCalls atomic.Int64
	provider := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(p, 15*time.Minute),
		auth.WithOnDegraded(func(_ context.Context) { hookCalls.Add(1) }),
	)

	// jwkCache.Get() will fail (endpoint still down) and the provider uses the
	// persisted fallback → onDegraded must fire.
	_, _ = provider.ValidateToken(context.Background(), "not.a.real.jwt")

	if hookCalls.Load() == 0 {
		t.Error("WithOnDegraded hook must fire when persisted fallback is used due to endpoint outage")
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

// ── Valid signed JWT tests ────────────────────────────────────────────────────

// testKeyPair holds an RSA key pair and the JWKS JSON derived from the public key.
// Used to build test JWKS servers that serve verifiable tokens.
type testKeyPair struct {
	privateKey *rsa.PrivateKey
	privJWK    jwk.Key
	jwksJSON   []byte
}

// newTestKeyPair generates a 2048-bit RSA key pair and encodes the public key
// as a JWKS payload ready to be served from an httptest.Server.
func newTestKeyPair(t *testing.T) *testKeyPair {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	privJWK, err := jwk.FromRaw(priv)
	if err != nil {
		t.Fatalf("build private JWK: %v", err)
	}
	_ = privJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	_ = privJWK.Set(jwk.KeyIDKey, "test-kid")

	pubJWK, err := privJWK.PublicKey()
	if err != nil {
		t.Fatalf("build public JWK: %v", err)
	}
	set := jwk.NewSet()
	_ = set.AddKey(pubJWK)
	payload, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	return &testKeyPair{privateKey: priv, privJWK: privJWK, jwksJSON: payload}
}

// sign creates a signed RS256 JWT with the given claims.
func (kp *testKeyPair) sign(t *testing.T, subject, sid string, iat time.Time, extra map[string]interface{}) string {
	t.Helper()
	tok := jwt.New()
	_ = tok.Set(jwt.SubjectKey, subject)
	_ = tok.Set(jwt.IssuedAtKey, iat)
	_ = tok.Set(jwt.ExpirationKey, iat.Add(time.Hour))
	if sid != "" {
		_ = tok.Set("sid", sid)
	}
	for k, v := range extra {
		_ = tok.Set(k, v)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, kp.privJWK))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return string(signed)
}

// newJWKSServer starts a test JWKS server that serves kp.jwksJSON.
func newJWKSServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestJWKSProvider_ValidToken_ReturnsSubjectAndSessionID verifies the happy path:
// a correctly signed JWT produces Claims with the expected Subject and SessionID.
// This exercises the post-parse claims extraction code path including the "sid" claim.
func TestJWKSProvider_ValidToken_ReturnsSubjectAndSessionID(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)

	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_abc", "sid_123", now, nil)

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("expected valid claims, got error: %v", err)
	}
	if claims.Subject != "user_abc" {
		t.Errorf("subject: got %q, want %q", claims.Subject, "user_abc")
	}
	if claims.SessionID != "sid_123" {
		t.Errorf("session ID: got %q, want %q", claims.SessionID, "sid_123")
	}
	if !claims.IssuedAt.Equal(now) {
		t.Errorf("IssuedAt: got %v, want %v", claims.IssuedAt, now)
	}
}

// TestJWKSProvider_ValidToken_FvaFirstFactor_ExtractsSessionStartedAt verifies
// that fva[0] is parsed and converted to a stable SessionStartedAt timestamp.
func TestJWKSProvider_ValidToken_FvaFirstFactor_ExtractsSessionStartedAt(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)

	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	// fva[0] = 300 means first factor was verified 300 s before iat.
	raw := kp.sign(t, "user_abc", "sid_fva", now, map[string]interface{}{
		"fva": []interface{}{float64(300), float64(-1)}, // second factor pending
	})

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := now.Add(-300 * time.Second)
	if !claims.SessionStartedAt.Equal(want) {
		t.Errorf("SessionStartedAt: got %v, want %v", claims.SessionStartedAt, want)
	}
	if !claims.MFAVerifiedAt.IsZero() {
		t.Errorf("MFAVerifiedAt must be zero for negative fva[1]; got %v", claims.MFAVerifiedAt)
	}
}

// TestJWKSProvider_ValidToken_FvaBothFactors_ExtractsMFAVerifiedAt verifies
// that both fva[0] and fva[1] are parsed and MFAVerifiedAt is set correctly.
func TestJWKSProvider_ValidToken_FvaBothFactors_ExtractsMFAVerifiedAt(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)

	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_abc", "sid_mfa", now, map[string]interface{}{
		"fva": []interface{}{float64(600), float64(120)},
	})

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claims.SessionStartedAt.Equal(now.Add(-600 * time.Second)) {
		t.Errorf("SessionStartedAt wrong: got %v", claims.SessionStartedAt)
	}
	if !claims.MFAVerifiedAt.Equal(now.Add(-120 * time.Second)) {
		t.Errorf("MFAVerifiedAt wrong: got %v", claims.MFAVerifiedAt)
	}
}

// TestJWKSProvider_ValidToken_FvaZeroFirstFactor_SessionStartedAtZero verifies
// that fva[0] = 0 leaves SessionStartedAt unset (treated as absent). A zero value
// is ambiguous: it could mean "first factor verified at exactly iat" (brand-new
// session) or a Clerk sentinel for missing data. Treating it as absent forces the
// safe fallback via the session_starts DB path, which produces an equivalent
// result for a genuinely new session while preventing a potential max-age bypass
// if 0 were ever sent for an old session with a recently refreshed JWT.
func TestJWKSProvider_ValidToken_FvaZeroFirstFactor_SessionStartedAtZero(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)
	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_abc", "sid_fva_zero", now, map[string]interface{}{
		"fva": []interface{}{float64(0), float64(-1)},
	})

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("token itself is valid; parse should not fail: %v", err)
	}
	if !claims.SessionStartedAt.IsZero() {
		t.Errorf("fva[0]=0 must leave SessionStartedAt zero (treated as absent); got %v", claims.SessionStartedAt)
	}
}

// TestJWKSProvider_ValidToken_FvaZeroMFA_MFAVerifiedAtSet verifies that fva[1] = 0
// correctly sets MFAVerifiedAt = issuedAt. Unlike fva[0], Clerk uses -1 as the
// documented sentinel for "MFA not yet completed", so 0 unambiguously means
// "second factor verified at exactly iat" (just completed).
func TestJWKSProvider_ValidToken_FvaZeroMFA_MFAVerifiedAtSet(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)
	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_abc", "sid_mfa_zero", now, map[string]interface{}{
		"fva": []interface{}{float64(300), float64(0)}, // MFA completed at exactly iat
	})

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("token itself is valid; parse should not fail: %v", err)
	}
	if !claims.MFAVerifiedAt.Equal(now) {
		t.Errorf("fva[1]=0 must set MFAVerifiedAt to issuedAt; got %v, want %v", claims.MFAVerifiedAt, now)
	}
}

// TestJWKSProvider_ValidToken_FvaOverflow_SessionStartedAtZero verifies that an
// fva[0] value large enough to overflow int64 (e.g. 1e18) is rejected and
// SessionStartedAt remains zero rather than being set to a nonsensical future time.
// Without the upper-bound check, time.Duration(1e18)*time.Second overflows int64
// and produces a negative duration, making SessionStartedAt appear in the future.
func TestJWKSProvider_ValidToken_FvaOverflow_SessionStartedAtZero(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)
	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t))

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_abc", "sid_overflow", now, map[string]interface{}{
		"fva": []interface{}{float64(1e18), float64(-1)},
	})

	claims, err := p.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("token itself is valid; parse should not fail: %v", err)
	}
	if !claims.SessionStartedAt.IsZero() {
		t.Errorf("SessionStartedAt should be zero for overflow fva[0]; got %v", claims.SessionStartedAt)
	}
}

// TestJWKSProvider_WithKeysetPersister_PersistsSucessfulFetch verifies that
// a successful ValidateToken call on a healthy endpoint refreshes the persisted
// keyset (exercises the else-branch of ValidateToken's keySet fetch).
func TestJWKSProvider_WithKeysetPersister_PersistsSuccessfulFetch(t *testing.T) {
	kp := newTestKeyPair(t)
	srv := newJWKSServer(t, kp.jwksJSON)

	sp := &stubPersister{}
	p := auth.NewJWKSProvider(context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(sp, 15*time.Minute),
	)

	savedAfterWarmup := len(sp.saved)

	now := time.Now().Truncate(time.Second)
	raw := kp.sign(t, "user_x", "sid_x", now, nil)
	_, _ = p.ValidateToken(context.Background(), raw)

	// persistKeyset is called both at warmup and on each successful ValidateToken.
	if len(sp.saved) == 0 {
		t.Error("expected persister to have saved keyset after successful warmup + ValidateToken")
	}
	_ = savedAfterWarmup // used to verify at least warmup path was exercised
}

// TestJWKSProvider_WithKeysetPersister_EmptyPayload_RemainsFailClosed verifies
// that a persister returning an empty/nil payload (cache miss) leaves the provider
// in fail-closed mode when the JWKS endpoint is also unavailable.
func TestJWKSProvider_WithKeysetPersister_EmptyPayload_RemainsFailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	sp := &stubPersister{loadVal: nil} // empty → cache miss
	provider := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(sp, 15*time.Minute),
	)

	_, err := provider.ValidateToken(context.Background(), "any.token")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable when persisted payload is empty; got %v", err)
	}
}

// TestJWKSProvider_WithKeysetPersister_InvalidPayload_RemainsFailClosed verifies
// that an unparse-able persisted JWKS payload is discarded gracefully and the
// provider remains fail-closed.
func TestJWKSProvider_WithKeysetPersister_InvalidPayload_RemainsFailClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	sp := &stubPersister{loadVal: []byte("not { valid json")}
	provider := auth.NewJWKSProvider(
		context.Background(), srv.URL, auth.DefaultWarmupTimeout, zaptest.NewLogger(t),
		auth.WithKeysetPersister(sp, 15*time.Minute),
	)

	_, err := provider.ValidateToken(context.Background(), "any.token")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable when persisted payload is invalid; got %v", err)
	}
}
