package middleware_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/auth"
)

// stubProvider implements auth.IdentityProvider for policy tests.
type stubProvider struct {
	claims auth.Claims
	err    error
}

func (s *stubProvider) ValidateToken(_ context.Context, _ string) (auth.Claims, error) {
	return s.claims, s.err
}

// stubParams implements GetInter.
type stubParams struct{ maxAge int }

func (s *stubParams) GetInt(_ context.Context, _ string, def int) int {
	if s.maxAge == 0 {
		return def
	}
	return s.maxAge
}

// stubBlocklist implements IsRevoker.
type stubBlocklist struct {
	revoked bool
	err     error
}

func (s *stubBlocklist) IsRevoked(_ context.Context, _ string) (bool, error) {
	return s.revoked, s.err
}

func makePolicyProvider(inner auth.IdentityProvider, bl middleware.IsRevoker, maxAgeSecs int) auth.IdentityProvider {
	return middleware.NewPolicyProvider(inner, bl, &stubParams{maxAge: maxAgeSecs}, zap.NewNop())
}

func TestPolicyProvider_FreshToken_NotRevoked_Passes(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now(),
		SessionID: "sid_1",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		3600,
	)

	got, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.Subject != "user_abc" {
		t.Errorf("subject mismatch: got %q", got.Subject)
	}
}

func TestPolicyProvider_ExpiredToken_RejectsWithInvalidToken(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		SessionID: "sid_old",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		3600, // max_age = 1 hour; token is 2 hours old
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestPolicyProvider_RevokedSession_RejectsWithInvalidToken(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now(),
		SessionID: "sid_revoked",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: true},
		3600,
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for revoked session, got %v", err)
	}
}

func TestPolicyProvider_NoSID_SkipsBlocklistCheck(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now(),
		SessionID: "", // no sid — blocklist must not be called
	}
	bl := &stubBlocklist{revoked: true} // would reject if called
	p := makePolicyProvider(&stubProvider{claims: claims}, bl, 3600)

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected no error when sid is empty (blocklist skipped), got %v", err)
	}
}

func TestPolicyProvider_BlocklistError_FailsOpen(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now(),
		SessionID: "sid_1",
	}
	bl := &stubBlocklist{revoked: false, err: errors.New("redis: connection refused")}
	p := makePolicyProvider(&stubProvider{claims: claims}, bl, 3600)

	// Blocklist error must not reject the request (fail-open).
	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected fail-open on blocklist error, got %v", err)
	}
}

func TestPolicyProvider_NilBlocklist_Passes(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_abc",
		IssuedAt:  time.Now(),
		SessionID: "sid_1",
	}
	p := middleware.NewPolicyProvider(&stubProvider{claims: claims}, nil, &stubParams{maxAge: 3600}, zap.NewNop())

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("nil blocklist must not reject requests, got %v", err)
	}
}

func TestPolicyProvider_InnerProviderError_Propagated(t *testing.T) {
	p := makePolicyProvider(
		&stubProvider{err: auth.ErrProviderUnavailable},
		nil,
		3600,
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrProviderUnavailable) {
		t.Fatalf("expected inner ErrProviderUnavailable to propagate, got %v", err)
	}
}

// ── SessionStartedAt (fva-derived) max-age tests ─────────────────────────────

// TestPolicyProvider_OldSession_FreshJWT_RejectsWithInvalidToken is the
// regression test for the bug where sessions older than max-age were NOT
// rejected because IssuedAt was used instead of SessionStartedAt.
// In production, Clerk refreshes JWTs every ~60 s: IssuedAt is always recent
// even for a 24-hour-old session. SessionStartedAt (derived from fva[0]) is
// stable across refreshes and correctly reflects the session origin.
func TestPolicyProvider_OldSession_FreshJWT_RejectsWithInvalidToken(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_abc",
		IssuedAt:         time.Now(),                     // JWT just refreshed — very recent
		SessionStartedAt: time.Now().Add(-6 * time.Hour), // but session started 6 h ago
		SessionID:        "sid_old_session",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		18000, // max_age = 5 hours; session is 6 h old → must reject
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for session older than max-age, got %v", err)
	}
}

// TestPolicyProvider_FreshSession_FreshJWT_Passes verifies that a session
// within the max-age window is not rejected, even with SessionStartedAt set.
func TestPolicyProvider_FreshSession_FreshJWT_Passes(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_abc",
		IssuedAt:         time.Now(),
		SessionStartedAt: time.Now().Add(-1 * time.Hour), // 1 h old, within 5 h max-age
		SessionID:        "sid_active",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		18000, // max_age = 5 hours
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected no error for active session within max-age, got %v", err)
	}
}

// TestPolicyProvider_ZeroSessionStartedAt_FallsBackToIssuedAt verifies that
// tokens without fva (e.g. test tokens, non-Clerk providers) fall back to
// IssuedAt for max-age enforcement so existing behaviour is preserved.
func TestPolicyProvider_ZeroSessionStartedAt_FallsBackToIssuedAt(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_abc",
		IssuedAt:         time.Now().Add(-2 * time.Hour), // old JWT, no fva
		SessionStartedAt: time.Time{},                    // zero → fallback
		SessionID:        "sid_no_fva",
	}
	p := makePolicyProvider(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		3600, // max_age = 1 h; IssuedAt is 2 h ago → must reject
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken when falling back to IssuedAt, got %v", err)
	}
}

// ── DB-backed session-start tracking (OAuth / fva-absent flows) ──────────────

// stubSessionStarter implements SessionStarter for policy tests.
type stubSessionStarter struct {
	startedAt time.Time
	err       error
	calls     int
}

func (s *stubSessionStarter) UpsertSessionStart(_ context.Context, _ string) (time.Time, error) {
	s.calls++
	return s.startedAt, s.err
}

func makePolicyProviderWithStarter(
	inner auth.IdentityProvider,
	bl middleware.IsRevoker,
	ss middleware.SessionStarter,
	maxAgeSecs int,
) auth.IdentityProvider {
	return middleware.NewPolicyProvider(inner, bl, &stubParams{maxAge: maxAgeSecs}, zap.NewNop(),
		middleware.WithSessionStarter(ss),
	)
}

// TestPolicyProvider_DBSession_OldSession_Rejects verifies that when fva is absent
// and the DB tracker returns a start time older than max-age, the token is rejected.
// This is the primary regression test for the OAuth session-timeout bug.
func TestPolicyProvider_DBSession_OldSession_Rejects(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_oauth",
		IssuedAt:         time.Now(), // JWT just refreshed — very recent
		SessionStartedAt: time.Time{}, // fva absent (OAuth login)
		SessionID:        "sid_oauth_old",
	}
	ss := &stubSessionStarter{
		startedAt: time.Now().Add(-6 * time.Hour), // session started 6 h ago
	}
	p := makePolicyProviderWithStarter(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		ss,
		18000, // max_age = 5 h; session is 6 h old → must reject
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for OAuth session older than max-age, got %v", err)
	}
	if ss.calls != 1 {
		t.Errorf("expected UpsertSessionStart called once, got %d", ss.calls)
	}
}

// TestPolicyProvider_DBSession_FreshSession_Passes verifies that a fresh OAuth
// session (within max-age) passes despite fva being absent.
func TestPolicyProvider_DBSession_FreshSession_Passes(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_oauth",
		IssuedAt:         time.Now(),
		SessionStartedAt: time.Time{}, // fva absent
		SessionID:        "sid_oauth_fresh",
	}
	ss := &stubSessionStarter{
		startedAt: time.Now().Add(-1 * time.Hour), // 1 h old, within 5 h max-age
	}
	p := makePolicyProviderWithStarter(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		ss,
		18000,
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected no error for fresh OAuth session, got %v", err)
	}
}

// TestPolicyProvider_DBSession_StarterError_FallsBackToIssuedAt verifies that
// when UpsertSessionStart fails the provider falls back to IssuedAt (fail-open),
// preserving availability during a DB outage.
func TestPolicyProvider_DBSession_StarterError_FallsBackToIssuedAt(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_oauth",
		IssuedAt:         time.Now(), // fresh JWT — within max-age via IssuedAt
		SessionStartedAt: time.Time{},
		SessionID:        "sid_oauth_dberr",
	}
	ss := &stubSessionStarter{err: errors.New("connection refused")}
	p := makePolicyProviderWithStarter(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		ss,
		18000,
	)

	// DB error + fresh IssuedAt → should pass (fail-open)
	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected fail-open on DB error, got %v", err)
	}
}

// TestPolicyProvider_FvaPresent_StarterNotCalled confirms that when fva is present
// (SessionStartedAt != zero) the DB tracker is not consulted.
func TestPolicyProvider_FvaPresent_StarterNotCalled(t *testing.T) {
	claims := auth.Claims{
		Subject:          "user_abc",
		IssuedAt:         time.Now(),
		SessionStartedAt: time.Now().Add(-1 * time.Hour), // fva present
		SessionID:        "sid_fva",
	}
	ss := &stubSessionStarter{startedAt: time.Now()} // would pass if called, but must NOT be called
	p := makePolicyProviderWithStarter(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		ss,
		18000,
	)

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ss.calls != 0 {
		t.Errorf("expected UpsertSessionStart not called when fva is present, got %d calls", ss.calls)
	}
}
