package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// GroupHandler handles HTTP requests for the /api/v1/groups resource.
type GroupHandler struct {
	quinielaSvc service.QuinielaService
	memberSvc   service.GroupMembershipService
	log         *zap.Logger
}

// NewGroupHandler constructs a GroupHandler.
func NewGroupHandler(
	quinielaSvc service.QuinielaService,
	memberSvc service.GroupMembershipService,
	log *zap.Logger,
) *GroupHandler {
	return &GroupHandler{quinielaSvc: quinielaSvc, memberSvc: memberSvc, log: log}
}

// renameGroupRequest is the JSON body accepted by PATCH /api/v1/groups/{id}.
type renameGroupRequest struct {
	Name string `json:"name"`
}

// createGroupRequest is the JSON body accepted by POST /api/v1/groups.
type createGroupRequest struct {
	Name           string `json:"name"`
	EntryFee       int    `json:"entry_fee"`
	Currency       string `json:"currency"`
	MaxMembers     *int   `json:"max_members"`
	PrizeThreshold int    `json:"prize_threshold"` // optional; defaults to DefaultPrizeThreshold when 0
}

// joinGroupRequest is the JSON body accepted by POST /api/v1/groups/join.
type joinGroupRequest struct {
	InviteCode string `json:"invite_code"`
}

// Create handles POST /api/v1/groups.
//
// @Summary      Create a group
// @Description  Creates a new prediction group. The caller becomes the owner
//
//	and is automatically added as an active member.
//
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.createGroupRequest  true  "Group details"
// @Success      201   {object}  handler.GroupResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      409   {object}  handler.ErrorResponse  "Group name already taken"
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/groups [post]
func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[createGroupRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	quiniela := &domain.Quiniela{
		Name:           req.Name,
		OwnerID:        caller.ID,
		EntryFee:       req.EntryFee,
		Currency:       req.Currency,
		MaxMembers:     req.MaxMembers,
		PrizeThreshold: req.PrizeThreshold, // 0 → service applies DefaultPrizeThreshold
	}
	if err := h.quinielaSvc.Create(r.Context(), quiniela); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, groupToResponse(quiniela))
}

// GetByID handles GET /api/v1/groups/{id}.
//
// @Summary      Get a group
// @Description  Returns group metadata by ID.
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Group ID"
// @Success      200  {object}  handler.GroupResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id} [get]
func (h *GroupHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	quiniela, err := h.quinielaSvc.GetByID(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, groupToResponse(quiniela))
}

// Join handles POST /api/v1/groups/join.
//
// @Summary      Join a group
// @Description  Joins the group identified by the invite code. The caller becomes
//
//	an active member. Re-joining after leaving is supported.
//
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.joinGroupRequest  true  "Invite code"
// @Success      200   {object}  handler.MemberResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      404   {object}  handler.ErrorResponse  "Invite code not found"
// @Failure      409   {object}  handler.ErrorResponse  "Already a member or group is full"
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/groups/join [post]
func (h *GroupHandler) Join(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[joinGroupRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	if req.InviteCode == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invite_code is required"))
		return
	}

	membership, err := h.memberSvc.Join(r.Context(), req.InviteCode, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberToResponse(membership))
}

// ListMembers handles GET /api/v1/groups/{id}/members.
//
// @Summary      List group members
// @Description  Returns all memberships for the given group.
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Group ID"
// @Success      200  {array}   handler.MemberResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/members [get]
func (h *GroupHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	members, err := h.memberSvc.ListByQuiniela(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	out := make([]MemberResponse, len(members))
	for i, m := range members {
		out[i] = memberToResponse(m)
	}
	writeJSON(w, http.StatusOK, out)
}

// ApproveJoin handles POST /api/v1/groups/{id}/members/{membershipID}/approve.
//
// @Summary      Approve a join request
// @Description  Promotes a pending membership to active. Any active member of
//
//	the group may approve — there is no admin-only restriction.
//	After approval the group status is re-evaluated: the group
//	transitions to "active" when it reaches the minimum active-member
//	threshold.
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id           path      int  true  "Group ID"
// @Param        membershipID path      int  true  "Membership ID to approve"
// @Success      200          {object}  handler.MemberResponse
// @Failure      401          {object}  handler.ErrorResponse
// @Failure      403          {object}  handler.ErrorResponse  "Caller is not an active member of this group"
// @Failure      404          {object}  handler.ErrorResponse  "Join request not found"
// @Failure      409          {object}  handler.ErrorResponse  "Request is no longer pending"
// @Failure      422          {object}  handler.ErrorResponse
// @Failure      500          {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/members/{membershipID}/approve [post]
func (h *GroupHandler) ApproveJoin(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	quinielaID, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	membershipID, err := pathID(r, "membershipID")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	membership, err := h.memberSvc.ApproveJoin(r.Context(), quinielaID, membershipID, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberToResponse(membership))
}

// Leave handles DELETE /api/v1/groups/{id}/members/me.
//
// @Summary      Leave a group
// @Description  Removes the authenticated user from the group. Only the user
//
//	themselves may leave — no admin or owner can remove another
//	member. After leaving, the group status is re-evaluated and may
//	become "inactive" if the active member count falls below the
//	minimum threshold.
//
// @Tags         groups
// @Security     BearerAuth
// @Param        id  path  int  true  "Group ID"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      422  {object}  handler.ErrorResponse  "Caller is not an active member of this group"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/members/me [delete]
func (h *GroupHandler) Leave(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	quinielaID, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	if err := h.memberSvc.Leave(r.Context(), quinielaID, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RenameGroup handles PATCH /api/v1/groups/{id}.
//
// @Summary      Rename a group
// @Description  Updates the name of the group. Only the CreateOwner (the member
//
//	with MembershipRoleCreateOwner) may rename their own group.
//
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                        true  "Group ID"
// @Param        body  body      handler.renameGroupRequest true  "New group name"
// @Success      200   {object}  handler.GroupResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not the group owner"
// @Failure      404   {object}  handler.ErrorResponse
// @Failure      409   {object}  handler.ErrorResponse  "Name already taken"
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id} [patch]
func (h *GroupHandler) RenameGroup(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	req, err := decodeJSON[renameGroupRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	quiniela, err := h.quinielaSvc.RenameGroup(r.Context(), id, caller.ID, req.Name)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, groupToResponse(quiniela))
}

// ListMyGroups handles GET /api/v1/groups/me.
//
// @Summary      List my groups
// @Description  Returns all groups the authenticated user belongs to.
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.MemberResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/me [get]
func (h *GroupHandler) ListMyGroups(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	members, err := h.memberSvc.ListByUser(r.Context(), caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	out := make([]MemberResponse, len(members))
	for i, m := range members {
		out[i] = memberToResponse(m)
	}
	writeJSON(w, http.StatusOK, out)
}
