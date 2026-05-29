package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// UserHandler handles user-facing profile operations.
type UserHandler struct {
	userRepo repository.UserRepository
	log      *zap.Logger
}

// NewUserHandler constructs a UserHandler.
func NewUserHandler(userRepo repository.UserRepository, log *zap.Logger) *UserHandler {
	return &UserHandler{userRepo: userRepo, log: log}
}

// GetMe handles GET /api/v1/users/me.
//
// Returns the authenticated user's own profile including their locale preference.
// Requires ResolveUser middleware (user is already in context from the /users subrouter).
//
// @Summary     Get own user profile
// @Tags        users
// @Produce     json
// @Success     200  {object}  handler.MeResponse
// @Failure     401  {object}  handler.ErrorResponse
// @Router      /api/v1/users/me [get]
func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	writeJSON(w, http.StatusOK, meToResponse(caller))
}

// updateMeRequest is the body accepted by PATCH /api/v1/users/me.
type updateMeRequest struct {
	Locale *string `json:"locale"`
}

// UpdateMe handles PATCH /api/v1/users/me.
// Currently supports updating the locale preference ("en" or "es").
//
// @Summary     Update user profile preferences
// @Tags        users
// @Accept      json
// @Produce     json
// @Param       body  body  updateMeRequest  true  "Fields to update"
// @Success     204
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     422   {object}  ErrorResponse
// @Router      /api/v1/users/me [patch]
func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised("authentication required"))
		return
	}

	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, apperrors.BadRequest("invalid request body"))
		return
	}

	if req.Locale != nil {
		locale := domain.ParseLocale(*req.Locale)
		// Reject unsupported locale tags that ParseLocale fell back to the
		// default for, but the original string does not match en or es.
		raw := *req.Locale
		if raw != string(domain.LocaleEN) && raw != string(domain.LocaleES) {
			middleware.WriteError(w, r, h.log,
				apperrors.Validation("locale must be \"en\" or \"es\""))
			return
		}
		if err := h.userRepo.UpdateLocale(r.Context(), caller.ID, string(locale)); err != nil {
			middleware.WriteError(w, r, h.log, err)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
