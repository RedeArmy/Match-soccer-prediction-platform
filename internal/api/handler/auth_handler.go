package handler

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// SessionRevoker records a local revocation for a Clerk session.
// PostgresSessionRepository satisfies this interface.
type SessionRevoker interface {
	RevokeSession(ctx context.Context, sid, userID string) error
}

// AuthHandler handles local session lifecycle endpoints.
type AuthHandler struct {
	sessions SessionRevoker
	log      *zap.Logger
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(sessions SessionRevoker, log *zap.Logger) *AuthHandler {
	return &AuthHandler{sessions: sessions, log: log}
}

// Logout handles POST /api/v1/auth/logout.
//
// Inserts the current session's "sid" into the local revocation blocklist so
// that any subsequent request carrying the same Clerk JWT is rejected with 401
// by PolicyProvider — even before Clerk's own session expiry fires.
//
// The frontend must also call Clerk's signOut() to clear the browser-side token
// and prevent it from being sent on future requests. Both operations together
// (backend revocation + Clerk sign-out) constitute a complete logout.
//
// Returns 204 No Content on success. Returns 204 without action when the JWT
// does not carry a "sid" claim (e.g. test tokens issued without a real Clerk
// session), because there is nothing to revoke in that case.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	sid, ok := middleware.SessionIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID, _ := middleware.UserIDFromContext(r.Context())

	if err := h.sessions.RevokeSession(r.Context(), sid, userID); err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
