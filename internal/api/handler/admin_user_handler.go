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
//
// @Summary      List users
// @Description  Returns a paginated list of all user accounts. Supports optional
//
//	filtering by ban status, role, and name/email search. Requires admin role.
//
// @Tags         admin-users
// @Produce      json
// @Security     BearerAuth
// @Param        banned  query     bool    false  "Filter by ban status (true = banned only, false = active only)"
// @Param        role    query     string  false  "Filter by role (admin, player)"
// @Param        search  query     string  false  "Search by name or email (partial match)"
// @Param        limit   query     int     false  "Max records per page (default 50, max 200)"
// @Param        page    query     int     false  "Page number (default 1)"
// @Success      200     {object}  handler.Paged[handler.AdminUserResponse]
// @Failure      401     {object}  handler.ErrorResponse
// @Failure      403     {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500     {object}  handler.ErrorResponse
// @Router       /api/v1/admin/users [get]
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
//
// @Summary      Get user profile
// @Description  Returns the full admin view of a user account: base profile, group
//
//	memberships, and payment records. Requires admin role.
//
// @Tags         admin-users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  handler.AdminUserProfileResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "User not found"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid user ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/users/{id} [get]
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
//
// @Summary      Ban a user
// @Description  Suspends a user account. Banned users are blocked from all write
//
//	operations. If the user owns any groups, ownership is automatically
//	transferred to the oldest active member. Requires admin role.
//
// @Tags         admin-users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                       true  "User ID"
// @Param        body  body      handler.banRequest        true  "Ban reason (required)"
// @Success      200   {object}  handler.AdminUserResponse
// @Failure      400   {object}  handler.ErrorResponse  "reason is required"
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404   {object}  handler.ErrorResponse  "User not found"
// @Failure      422   {object}  handler.ErrorResponse  "Invalid user ID"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/users/{id}/ban [post]
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
//
// @Summary      Unban a user
// @Description  Lifts a suspension from a user account, restoring full access.
//
//	Requires admin role.
//
// @Tags         admin-users
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  handler.AdminUserResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "User not found"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid user ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/users/{id}/ban [delete]
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
//
// @Summary      Bulk-ban users
// @Description  Bans multiple user accounts in a single request. Each ban is
//
//	processed sequentially; a failure on one user is logged and
//	skipped so the remaining users are still banned. Requires admin role.
//
// @Tags         admin-users
// @Accept       json
// @Security     BearerAuth
// @Param        body  body  handler.bulkBanRequest  true  "List of user IDs and ban reason"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "user_ids or reason missing"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/users/bulk-ban [post]
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
