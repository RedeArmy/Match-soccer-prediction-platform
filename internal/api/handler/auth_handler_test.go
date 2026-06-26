package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// stubSessionRevoker captures calls to RevokeSession for assertion.
type stubSessionRevoker struct {
	calledSID    string
	calledUserID string
	err          error
}

func (s *stubSessionRevoker) RevokeSession(_ context.Context, sid, userID string) error {
	s.calledSID = sid
	s.calledUserID = userID
	return s.err
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
