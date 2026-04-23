package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const msgInvalidUserID = "invalid user id"

// AdminUserHandler handles admin endpoints for user management.
type AdminUserHandler struct {
	svc service.AdminUserService
	log *zap.Logger
}

// NewAdminUserHandler constructs an AdminUserHandler.
func NewAdminUserHandler(svc service.AdminUserService, log *zap.Logger) *AdminUserHandler {
	return &AdminUserHandler{svc: svc, log: log}
}

// ListUsers handles GET /admin/users — paginated user list with optional filters.
func (h *AdminUserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	f := repository.UserFilters{}
	if q := r.URL.Query().Get("banned"); q != "" {
		b := q == "true"
		f.Banned = &b
	}
	if q := r.URL.Query().Get("role"); q != "" {
		role := domain.UserRole(q)
		f.Role = &role
	}
	if q := r.URL.Query().Get("search"); q != "" {
		f.Search = &q
	}

	users, err := h.svc.ListFiltered(r.Context(), f, p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]AdminUserResponse, len(users))
	for i, u := range users {
		data[i] = adminUserToResponse(u)
	}
	writeJSON(w, http.StatusOK, Paged[AdminUserResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}

// GetUserProfile handles GET /admin/users/{id} — full user profile.
func (h *AdminUserHandler) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidUserID))
		return
	}

	profile, err := h.svc.GetProfile(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	memberships := make([]MemberResponse, len(profile.Memberships))
	for i, m := range profile.Memberships {
		memberships[i] = memberToResponse(m)
	}
	payments := make([]PaymentResponse, len(profile.Payments))
	for i, p := range profile.Payments {
		payments[i] = paymentToResponse(p)
	}

	writeJSON(w, http.StatusOK, AdminUserProfileResponse{
		User:        adminUserToResponse(profile.User),
		Memberships: memberships,
		Payments:    payments,
	})
}

type banRequest struct {
	Reason string `json:"reason"`
}

// BanUser handles POST /admin/users/{id}/ban.
func (h *AdminUserHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidUserID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req banRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	banned, err := h.svc.BanUser(r.Context(), id, caller.ID, req.Reason)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserToResponse(banned))
}

// UnbanUser handles DELETE /admin/users/{id}/ban.
func (h *AdminUserHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidUserID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	user, err := h.svc.UnbanUser(r.Context(), id, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserToResponse(user))
}

type bulkBanRequest struct {
	UserIDs []int  `json:"user_ids"`
	Reason  string `json:"reason"`
}

// BulkBan handles POST /admin/users/bulk-ban.
func (h *AdminUserHandler) BulkBan(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req bulkBanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}
	if len(req.UserIDs) == 0 || req.Reason == "" {
		middleware.WriteError(w, r, h.log, decodeError(nil))
		return
	}

	if err := h.svc.BulkBan(r.Context(), req.UserIDs, caller.ID, req.Reason); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
