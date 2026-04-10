package handler

import (
	"encoding/json"
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

// createGroupRequest is the JSON body accepted by POST /api/v1/groups.
type createGroupRequest struct {
	Name       string `json:"name"`
	EntryFee   int    `json:"entry_fee"`
	Currency   string `json:"currency"`
	MaxMembers *int   `json:"max_members"`
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

	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	quiniela := &domain.Quiniela{
		Name:       req.Name,
		OwnerID:    caller.ID,
		EntryFee:   req.EntryFee,
		Currency:   req.Currency,
		MaxMembers: req.MaxMembers,
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

	var req joinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
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
