package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgInvalidGroupID = "invalid group id"
)

// AdminGroupHandler handles admin endpoints for group management.
type AdminGroupHandler struct {
	svc    service.AdminGroupService
	params service.SystemParamService
	log    *zap.Logger
}

// NewAdminGroupHandler constructs an AdminGroupHandler.
func NewAdminGroupHandler(svc service.AdminGroupService, params service.SystemParamService, log *zap.Logger) *AdminGroupHandler {
	return &AdminGroupHandler{svc: svc, params: params, log: log}
}

// DeleteGroup handles DELETE /admin/groups/{id}.
//
// @Summary      Delete a group
// @Description  Soft-deletes the quiniela group. All memberships and predictions
//
//	are preserved for audit purposes. Requires admin role.
//
// @Tags         admin-groups
// @Security     BearerAuth
// @Param        id  path  int  true  "Group ID"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Group not found"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid group ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id} [delete]
func (h *AdminGroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidGroupID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	if err := h.svc.DeleteGroup(r.Context(), id, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember handles DELETE /admin/groups/{id}/members/{membershipID}.
//
// @Summary      Remove a member
// @Description  Sets the given membership to the 'left' status, effectively
//
//	removing the user from the group. Unlike the self-removal endpoint,
//	this can target any member. Requires admin role.
//
// @Tags         admin-groups
// @Security     BearerAuth
// @Param        id            path  int  true  "Group ID"
// @Param        membershipID  path  int  true  "Membership ID to remove"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Membership not found or already inactive"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid membership ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/members/{membershipID} [delete]
func (h *AdminGroupHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	membershipID, err := strconv.Atoi(chi.URLParam(r, "membershipID"))
	if err != nil || membershipID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid membership id"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	if err := h.svc.RemoveMember(r.Context(), membershipID, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateGroupSettingsRequest struct {
	MaxMembers *int `json:"max_members"`
	EntryFee   *int `json:"entry_fee"`
}

// UpdateGroupSettings handles PATCH /admin/groups/{id}/settings.
//
// @Summary      Update group settings
// @Description  Changes the max_members cap and entry_fee of a group atomically.
//
//	Passing null for max_members removes the cap entirely. Requires admin role.
//
// @Tags         admin-groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                                   true  "Group ID"
// @Param        body  body      handler.updateGroupSettingsRequest    true  "Settings to update (entry_fee required)"
// @Success      200   {object}  handler.GroupResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404   {object}  handler.ErrorResponse  "Group not found"
// @Failure      422   {object}  handler.ErrorResponse  "entry_fee is required"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/settings [patch]
func (h *AdminGroupHandler) UpdateGroupSettings(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidGroupID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[updateGroupSettingsRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	if req.EntryFee == nil {
		middleware.WriteError(w, r, h.log, apperrors.Validation("entry_fee is required"))
		return
	}

	q, err := h.svc.UpdateGroupSettings(r.Context(), id, req.MaxMembers, *req.EntryFee, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, groupToResponse(q))
}

type transferOwnershipRequest struct {
	NewOwnerUserID int `json:"new_owner_user_id"`
}

// TransferOwnership handles POST /admin/groups/{id}/transfer-ownership.
//
// @Summary      Transfer group ownership
// @Description  Assigns MembershipRoleCreateOwner to new_owner_user_id and demotes
//
//	the current owner to a regular member. The new owner must be an
//	active member of the group. Requires admin role.
//
// @Tags         admin-groups
// @Accept       json
// @Security     BearerAuth
// @Param        id    path  int                               true  "Group ID"
// @Param        body  body  handler.transferOwnershipRequest  true  "New owner user ID"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Group or new owner not found"
// @Failure      422  {object}  handler.ErrorResponse  "new_owner_user_id is required"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/transfer-ownership [post]
func (h *AdminGroupHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidGroupID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[transferOwnershipRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	if req.NewOwnerUserID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("new_owner_user_id is required"))
		return
	}

	if err := h.svc.TransferOwnership(r.Context(), id, req.NewOwnerUserID, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bulkGroupIDsRequest struct {
	GroupIDs []int `json:"group_ids"`
}

// BulkDeleteGroups handles POST /admin/groups/bulk-delete.
//
// @Summary      Bulk delete groups
// @Description  Soft-deletes multiple quiniela groups. Returns 200 when all
//
//	groups were deleted, 207 Multi-Status when some could not be
//	deleted (already deleted or not found). Requires admin role.
//
// @Tags         admin-groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.bulkGroupIDsRequest         true  "Group IDs to delete"
// @Success      200   {object}  handler.BulkOperationResultResponse
// @Success      207   {object}  handler.BulkOperationResultResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422   {object}  handler.ErrorResponse  "group_ids is required"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/bulk-delete [post]
func (h *AdminGroupHandler) BulkDeleteGroups(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[bulkGroupIDsRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	maxItems := h.params.GetInt(r.Context(), domain.ParamKeyAdminBulkMaxItems, domain.DefaultAdminBulkMaxItems)
	switch {
	case len(req.GroupIDs) == 0:
		middleware.WriteError(w, r, h.log, apperrors.Validation("group_ids must not be empty"))
		return
	case len(req.GroupIDs) > maxItems:
		middleware.WriteError(w, r, h.log, apperrors.Validation(fmt.Sprintf("group_ids must not exceed %d items", maxItems)))
		return
	}

	result, err := h.svc.BulkDeleteGroups(r.Context(), req.GroupIDs, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	status := http.StatusOK
	if len(result.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, bulkOperationResultToResponse(result))
}

type bulkMemberIDsRequest struct {
	MembershipIDs []int `json:"membership_ids"`
}

// BulkRemoveMembers handles POST /admin/groups/{id}/members/bulk-remove.
//
// @Summary      Bulk remove members
// @Description  Sets multiple memberships to 'left'. Returns 200 when all
//
//	were removed, 207 Multi-Status when some could not be removed
//	(already inactive or not found). Requires admin role.
//
// @Tags         admin-groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                                 true  "Group ID — only memberships belonging to this group are removed"
// @Param        body  body      handler.bulkMemberIDsRequest        true  "Membership IDs to remove"
// @Success      200   {object}  handler.BulkOperationResultResponse
// @Success      207   {object}  handler.BulkOperationResultResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422   {object}  handler.ErrorResponse  "membership_ids is required or exceeds limit"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/members/bulk-remove [post]
func (h *AdminGroupHandler) BulkRemoveMembers(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	quinielaID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidGroupID))
		return
	}

	req, err := decodeJSON[bulkMemberIDsRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	maxItems := h.params.GetInt(r.Context(), domain.ParamKeyAdminBulkMaxItems, domain.DefaultAdminBulkMaxItems)
	switch {
	case len(req.MembershipIDs) == 0:
		middleware.WriteError(w, r, h.log, apperrors.Validation("membership_ids must not be empty"))
		return
	case len(req.MembershipIDs) > maxItems:
		middleware.WriteError(w, r, h.log, apperrors.Validation(fmt.Sprintf("membership_ids must not exceed %d items", maxItems)))
		return
	}

	result, err := h.svc.BulkRemoveMembers(r.Context(), quinielaID, req.MembershipIDs, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	status := http.StatusOK
	if len(result.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, bulkOperationResultToResponse(result))
}

// RecalculateLeaderboard handles POST /admin/groups/{id}/leaderboard/recalculate.
//
// @Summary      Recalculate leaderboard
// @Description  Triggers an immediate leaderboard snapshot for the group.
//
//	Useful after manually correcting match results or membership
//	data. Returns the newly created snapshot. Requires admin role.
//
// @Tags         admin-groups
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Group ID"
// @Success      200  {object}  handler.SnapshotResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid group ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/leaderboard/recalculate [post]
func (h *AdminGroupHandler) RecalculateLeaderboard(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation(msgInvalidGroupID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	snap, err := h.svc.RecalculateLeaderboard(r.Context(), id, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshotToResponse(snap))
}
