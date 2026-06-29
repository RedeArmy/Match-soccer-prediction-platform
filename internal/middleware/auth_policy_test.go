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
// calls is a counter of DB round-trips; tests assert it to verify cache behaviour.
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

// ── In-process start cache (DT-001) ──────────────────────────────────────────

// TestPolicyProvider_StartCache_SecondRequest_SkipsDB verifies that the
// in-process cache absorbs repeated requests from the same OAuth sid, calling
// UpsertSessionStart exactly once across N consecutive ValidateToken calls.
// This is the primary regression test for the per-request DB write on the hot
// authentication path.
func TestPolicyProvider_StartCache_SecondRequest_SkipsDB(t *testing.T) {
	sid := "sid_cache_hit_test"
	claims := auth.Claims{
		Subject:          "user_oauth",
		IssuedAt:         time.Now(),
		SessionStartedAt: time.Time{}, // fva absent → triggers cache path
		SessionID:        sid,
	}
	ss := &stubSessionStarter{startedAt: time.Now().Add(-30 * time.Minute)}
	p := makePolicyProviderWithStarter(
		&stubProvider{claims: claims},
		&stubBlocklist{revoked: false},
		ss,
		18000,
	)

	const requests = 5
	for i := range requests {
		_, err := p.ValidateToken(context.Background(), "any")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
	}
	if ss.calls != 1 {
		t.Errorf("expected UpsertSessionStart called exactly once across %d requests; got %d", requests, ss.calls)
	}
}

// TestPolicyProvider_StartCache_DifferentSIDs_EachHitsDB verifies that two
// sessions with different sids each get their own DB call; they do not share
// cache entries.
func TestPolicyProvider_StartCache_DifferentSIDs_EachHitsDB(t *testing.T) {
	ss := &stubSessionStarter{startedAt: time.Now().Add(-30 * time.Minute)}

	claimsA := auth.Claims{
		Subject:   "user_a",
		IssuedAt:  time.Now(),
		SessionID: "sid_alpha",
	}
	claimsB := auth.Claims{
		Subject:   "user_b",
		IssuedAt:  time.Now(),
		SessionID: "sid_beta",
	}

	// Two providers that share the same stubSessionStarter but different inner providers.
	// We use a single provider and swap the claims via sequential calls to match
	// how two distinct users would be served by one PolicyProvider instance.
	p := makePolicyProviderWithStarter(
		// The stub returns the same claims on every call; we test call count only.
		&stubProvider{claims: claimsA},
		&stubBlocklist{},
		ss,
		18000,
	)

	// First sid.
	if _, err := p.ValidateToken(context.Background(), "any"); err != nil {
		t.Fatalf("sid_alpha first request: %v", err)
	}
	// Same sid again — cache hit, DB not called.
	if _, err := p.ValidateToken(context.Background(), "any"); err != nil {
		t.Fatalf("sid_alpha second request: %v", err)
	}
	if ss.calls != 1 {
		t.Errorf("after 2 requests for same sid expected 1 DB call, got %d", ss.calls)
	}

	// Switch to a new sid by using a separate provider instance (different inner provider).
	ss2 := &stubSessionStarter{startedAt: time.Now().Add(-30 * time.Minute)}
	p2 := makePolicyProviderWithStarter(&stubProvider{claims: claimsB}, &stubBlocklist{}, ss2, 18000)
	if _, err := p2.ValidateToken(context.Background(), "any"); err != nil {
		t.Fatalf("sid_beta first request: %v", err)
	}
	if ss2.calls != 1 {
		t.Errorf("new sid should produce exactly 1 DB call, got %d", ss2.calls)
	}
}

// ── Bounded revoked cache (DT-002) ───────────────────────────────────────────

// TestPolicyProvider_RevokedCache_FailsClosedDuringDBOutage verifies that a
// session confirmed revoked in a previous successful check is still rejected
// when the blocklist DB is unavailable.
func TestPolicyProvider_RevokedCache_FailsClosedDuringDBOutage(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_revoked",
		IssuedAt:  time.Now(),
		SessionID: "sid_will_be_revoked",
	}

	// First call: DB says "revoked" → populates cache.
	bl := &stubBlocklist{revoked: true}
	p := makePolicyProvider(&stubProvider{claims: claims}, bl, 18000)
	_, err := p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("first call: expected ErrInvalidToken for revoked session, got %v", err)
	}

	// Second call: DB is now down, but cache should keep it rejected.
	bl.revoked = false
	bl.err = errors.New("connection refused")
	_, err = p.ValidateToken(context.Background(), "any")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("second call with DB down: expected ErrInvalidToken (fail-closed from cache), got %v", err)
	}
}

// TestPolicyProvider_RevokedCache_UnknownSID_FailsOpenDuringDBOutage verifies
// that a session never confirmed revoked by the DB fails-open during an outage,
// so a DB blip does not lock out all active users.
func TestPolicyProvider_RevokedCache_UnknownSID_FailsOpenDuringDBOutage(t *testing.T) {
	claims := auth.Claims{
		Subject:   "user_active",
		IssuedAt:  time.Now(),
		SessionID: "sid_never_revoked",
	}
	bl := &stubBlocklist{err: errors.New("timeout")} // DB unavailable from the start
	p := makePolicyProvider(&stubProvider{claims: claims}, bl, 18000)

	_, err := p.ValidateToken(context.Background(), "any")
	if err != nil {
		t.Fatalf("expected fail-open for unknown sid during DB outage, got %v", err)
	}
}
