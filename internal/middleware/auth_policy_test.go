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

// stubParams implements IntGetter.
type stubParams struct{ maxAge int }

func (s *stubParams) GetInt(_ context.Context, _ string, def int) int {
	if s.maxAge == 0 {
		return def
	}
	return s.maxAge
}

// stubBlocklist implements SessionBlocklist.
type stubBlocklist struct {
	revoked bool
	err     error
}

func (s *stubBlocklist) IsRevoked(_ context.Context, _ string) (bool, error) {
	return s.revoked, s.err
}

func makePolicyProvider(inner auth.IdentityProvider, bl middleware.SessionBlocklist, maxAgeSecs int) auth.IdentityProvider {
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
