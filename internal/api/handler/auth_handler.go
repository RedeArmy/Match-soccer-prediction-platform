package handler

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// SessionRevoker records local revocations for Clerk sessions.
// PostgresSessionRepository satisfies this interface.
type SessionRevoker interface {
	RevokeSession(ctx context.Context, sid, userID string) error
	// RevokeAllUserSessions bulk-revokes all known sessions for userID.
	// Returns the number of sessions revoked.
	RevokeAllUserSessions(ctx context.Context, userID string) (int64, error)
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

// RevokeAll handles DELETE /api/v1/auth/sessions.
//
// Force-logs out all devices for the authenticated account by bulk-inserting
// every known session_starts row for the user into revoked_sessions. Also
// revokes the current session directly (covers fva-present sessions that may
// not have a session_starts entry).
//
// Use case: compromised account — the user (or admin acting on their behalf)
// wants to invalidate all active sessions immediately. After this call, any
// request carrying a previously-valid JWT for this account is rejected with 401.
//
// Returns 204 No Content on success. The response body contains the number of
// sessions revoked in a JSON envelope for diagnostic purposes.
func (h *AuthHandler) RevokeAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised("authentication required"))
		return
	}

	n, err := h.sessions.RevokeAllUserSessions(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}

	// Also revoke the current session so the caller's own JWT is immediately
	// invalidated, forcing them to re-authenticate.
	if sid, ok := middleware.SessionIDFromContext(r.Context()); ok {
		_ = h.sessions.RevokeSession(r.Context(), sid, userID)
		n++
	}

	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}
