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
// @Summary      Log out current session
// @Description  Inserts the current session's sid into the local revocation blocklist.
// @Description  The caller must also invoke Clerk's signOut() to clear the browser-side
// @Description  token; both steps together constitute a complete logout.
// @Tags         auth
// @Security     BearerAuth
// @Success      204  "Session revoked"
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/auth/logout [post]
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

// revokeAllResponse is the body returned by DELETE /api/v1/auth/sessions.
type revokeAllResponse struct {
	Revoked int64 `json:"revoked"`
}

// RevokeAll handles DELETE /api/v1/auth/sessions.
//
// @Summary      Revoke all sessions
// @Description  Force-logs out all devices for the authenticated account by bulk-revoking
// @Description  every known session. Use when an account is compromised to immediately
// @Description  invalidate all active JWTs. The caller's own session is also revoked.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.revokeAllResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/auth/sessions [delete]
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

	writeJSON(w, http.StatusOK, revokeAllResponse{Revoked: n})
}
