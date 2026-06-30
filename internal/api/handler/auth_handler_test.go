package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// stubSessionRevoker captures calls to RevokeSession/RevokeAllUserSessions for assertion.
type stubSessionRevoker struct {
	calledSID    string
	calledUserID string
	err          error
	revokeAllN   int64
	revokeAllErr error
}

func (s *stubSessionRevoker) RevokeSession(_ context.Context, sid, userID string) error {
	s.calledSID = sid
	s.calledUserID = userID
	return s.err
}

func (s *stubSessionRevoker) RevokeAllUserSessions(_ context.Context, _ string) (int64, error) {
	return s.revokeAllN, s.revokeAllErr
}

func newAuthHandler(t *testing.T, revoker handler.SessionRevoker) *handler.AuthHandler {
	t.Helper()
	return handler.NewAuthHandler(revoker, zaptest.NewLogger(t))
}

func doLogout(h *handler.AuthHandler, sid, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	ctx := middleware.ContextWithUserID(req.Context(), userID)
	if sid != "" {
		ctx = middleware.ContextWithSessionID(ctx, sid)
	}
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.Logout(w, req)
	return w
}

func TestAuthHandler_Logout_WithSID_Returns204AndRevokes(t *testing.T) {
	revoker := &stubSessionRevoker{}
	h := newAuthHandler(t, revoker)

	w := doLogout(h, "sess_abc123", "user_clerk_xyz")

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if revoker.calledSID != "sess_abc123" {
		t.Errorf("expected RevokeSession called with sid %q, got %q", "sess_abc123", revoker.calledSID)
	}
	if revoker.calledUserID != "user_clerk_xyz" {
		t.Errorf("expected RevokeSession called with userID %q, got %q", "user_clerk_xyz", revoker.calledUserID)
	}
}

func TestAuthHandler_Logout_NoSID_Returns204WithoutRevoking(t *testing.T) {
	revoker := &stubSessionRevoker{}
	h := newAuthHandler(t, revoker)

	w := doLogout(h, "", "user_clerk_xyz")

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if revoker.calledSID != "" {
		t.Errorf("RevokeSession must not be called when sid is absent, but got sid=%q", revoker.calledSID)
	}
}

func TestAuthHandler_Logout_RevokerError_Returns500(t *testing.T) {
	revoker := &stubSessionRevoker{err: errors.New("db unavailable")}
	h := newAuthHandler(t, revoker)

	w := doLogout(h, "sess_abc123", "user_clerk_xyz")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on revoker error, got %d", w.Code)
	}
}

// ── RevokeAll (DELETE /api/v1/auth/sessions) ────────────────────────────────

func doRevokeAll(h *handler.AuthHandler, userID, sid string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions", nil)
	ctx := middleware.ContextWithUserID(req.Context(), userID)
	if sid != "" {
		ctx = middleware.ContextWithSessionID(ctx, sid)
	}
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.RevokeAll(w, req)
	return w
}

func TestAuthHandler_RevokeAll_WithSID_Returns200WithCount(t *testing.T) {
	revoker := &stubSessionRevoker{revokeAllN: 3}
	h := newAuthHandler(t, revoker)

	w := doRevokeAll(h, "user_abc", "sess_current")

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// revokeAllN=3 + 1 for the current session = 4
	body := strings.TrimSpace(w.Body.String())
	if body != `{"revoked":4}` {
		t.Errorf("expected body {\"revoked\":4}, got %s", body)
	}
}

func TestAuthHandler_RevokeAll_NoUserID_Returns401(t *testing.T) {
	revoker := &stubSessionRevoker{}
	h := newAuthHandler(t, revoker)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions", nil)
	// No user ID in context.
	w := httptest.NewRecorder()
	h.RevokeAll(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no user ID in context, got %d", w.Code)
	}
}

func TestAuthHandler_RevokeAll_BulkError_Returns500(t *testing.T) {
	revoker := &stubSessionRevoker{revokeAllErr: errors.New("db error")}
	h := newAuthHandler(t, revoker)

	w := doRevokeAll(h, "user_abc", "sess_current")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on bulk revoke error, got %d", w.Code)
	}
}
