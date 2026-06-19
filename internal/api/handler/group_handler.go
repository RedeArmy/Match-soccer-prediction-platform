package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// GroupHandler handles HTTP requests for the /api/v1/groups resource.
type GroupHandler struct {
	quinielaSvc service.QuinielaService
	memberSvc   service.GroupMembershipService
	authz       service.GroupAuthz
	params      service.SystemParamService
	matchSvc    service.MatchService
	predRepo    repository.PredictionRepository
	log         *zap.Logger
}

// NewGroupHandler constructs a GroupHandler.
// authz enforces that only active group members may read group details and member lists.
// matchSvc and predRepo are used by GetLivePredictions; both may be nil in tests that
// do not exercise that endpoint.
func NewGroupHandler(
	quinielaSvc service.QuinielaService,
	memberSvc service.GroupMembershipService,
	authz service.GroupAuthz,
	params service.SystemParamService,
	matchSvc service.MatchService,
	predRepo repository.PredictionRepository,
	log *zap.Logger,
) *GroupHandler {
	return &GroupHandler{
		quinielaSvc: quinielaSvc,
		memberSvc:   memberSvc,
		authz:       authz,
		params:      params,
		matchSvc:    matchSvc,
		predRepo:    predRepo,
		log:         log,
	}
}

// renameGroupRequest is the JSON body accepted by PATCH /api/v1/groups/{id}.
type renameGroupRequest struct {
	Name string `json:"name"`
}

// createGroupRequest is the JSON body accepted by POST /api/v1/groups.
// Entry fee and currency are resolved from system_params; callers supply only the name.
type createGroupRequest struct {
	Name string `json:"name"`
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[createGroupRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	ctx := r.Context()
	currency := h.params.GetString(ctx, domain.ParamKeyGroupCurrency, domain.DefaultGroupCurrency)

	// Groups are always created as free (entry_fee=0, is_premium=false).
	// The owner may later upgrade to premium via PATCH /{id}/tournament-mode.
	quiniela := &domain.Quiniela{
		Name:     req.Name,
		OwnerID:  caller.ID,
		EntryFee: 0,
		Currency: currency,
	}
	if err := h.quinielaSvc.Create(r.Context(), quiniela); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, groupToResponse(quiniela))
}

// checkNameResponse is the JSON body returned by GET /api/v1/groups/check-name.
type checkNameResponse struct {
	Available bool `json:"available"`
}

// CheckName handles GET /api/v1/groups/check-name?name=...
//
// @Summary      Check group name availability
// @Description  Returns whether the given name is available for a new group (case-insensitive).
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        name  query     string  true  "Group name to check"
// @Success      200   {object}  handler.checkNameResponse
// @Failure      400   {object}  handler.ErrorResponse  "name query param missing"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/groups/check-name [get]
func (h *GroupHandler) CheckName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, r, h.log, apperrors.Validation("name query parameter is required"))
		return
	}
	available, err := h.quinielaSvc.IsNameAvailable(r.Context(), name, 0)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, checkNameResponse{Available: available})
}

// GetByID handles GET /api/v1/groups/{id}.
//
// @Summary      Get a group
// @Description  Returns group metadata by ID. Requires active group membership.
//
//	The response includes the current invite code, which must not leak
//	to non-members — anyone holding it can join the group unsolicited.
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Group ID"
// @Success      200  {object}  handler.GroupResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Not an active member of this group"
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id} [get]
func (h *GroupHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if _, ok := requireGroupMember(w, r, h.log, h.authz, id); !ok {
		return
	}

	quiniela, err := h.quinielaSvc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, r, h.log, err)
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[joinGroupRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.InviteCode == "" {
		writeError(w, r, h.log, apperrors.Validation("invite_code is required"))
		return
	}

	membership, err := h.memberSvc.Join(r.Context(), req.InviteCode, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberToResponse(membership))
}

// JoinWithBalance handles POST /api/v1/groups/join-with-balance.
//
// @Summary      Join group and pay entry fee from balance
// @Description  Joins the group identified by the invite code and immediately deducts the entry fee from the caller's available balance.
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.joinGroupRequest  true  "Invite code"
// @Success      200   {object}  handler.MemberResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      404   {object}  handler.ErrorResponse  "Invite code not found"
// @Failure      409   {object}  handler.ErrorResponse  "Insufficient balance or already a member"
// @Failure      422   {object}  handler.ErrorResponse
// @Router       /api/v1/groups/join-with-balance [post]
func (h *GroupHandler) JoinWithBalance(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[joinGroupRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.InviteCode == "" {
		writeError(w, r, h.log, apperrors.Validation("invite_code is required"))
		return
	}

	membership, err := h.memberSvc.JoinWithBalance(r.Context(), req.InviteCode, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberToResponse(membership))
}

// ListMembers handles GET /api/v1/groups/{id}/members.
//
// @Summary      List group members
// @Description  Returns all memberships for the given group. Requires active group membership.
//
//	Each membership record includes the paid status (whether the member has
//	paid their entry fee), which must only be visible to fellow members.
//	Supports optional pagination via ?limit and ?offset (defaults to unbounded).
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id      path   int  true   "Group ID"
// @Param        limit   query  int  false  "Maximum number of results to return (0 = unbounded)"
// @Param        offset  query  int  false  "Number of results to skip (default: 0)"
// @Success      200     {object}  handler.Paged[handler.MemberResponse]
// @Failure      401     {object}  handler.ErrorResponse
// @Failure      403     {object}  handler.ErrorResponse  "Not an active member of this group"
// @Failure      500     {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/members [get]
func (h *GroupHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if _, ok := requireGroupMember(w, r, h.log, h.authz, id); !ok {
		return
	}

	limit, offset := parsePaginationParams(r)

	members, err := h.memberSvc.ListByQuiniela(r.Context(), id)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	// Apply in-memory pagination (service layer does not support pagination yet).
	pagedMembers := applySlicePagination(members, limit, offset)

	out := make([]MemberResponse, len(pagedMembers))
	for i, m := range pagedMembers {
		out[i] = memberToResponse(m)
	}

	writeJSON(w, http.StatusOK, Paged[MemberResponse]{
		Data: out,
		Page: PageMeta{Limit: limit, Offset: offset},
	})
}

// ApproveJoin handles POST /api/v1/groups/{id}/members/{membershipID}/approve.
//
// @Summary      Approve a join request
// @Description  Promotes a pending membership to active. Any active member of
//
//	the group may approve - there is no admin-only restriction.
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	quinielaID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	membershipID, err := pathID(r, "membershipID")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	membership, err := h.memberSvc.ApproveJoin(r.Context(), quinielaID, membershipID, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, memberToResponse(membership))
}

// RejectJoin handles DELETE /api/v1/groups/{id}/members/{membershipID}.
//
// @Summary      Reject a join request
// @Description  Sets a pending join request to left. Any active member of the group may reject.
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id           path  int  true  "Group ID"
// @Param        membershipID path  int  true  "Membership ID to reject"
// @Success      204  "Rejected"
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Not an active member of this group"
// @Failure      404  {object}  handler.ErrorResponse  "Join request not found"
// @Failure      409  {object}  handler.ErrorResponse  "Request is no longer pending"
// @Router       /api/v1/groups/{id}/members/{membershipID} [delete]
func (h *GroupHandler) RejectJoin(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	quinielaID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	membershipID, err := pathID(r, "membershipID")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if err := h.memberSvc.RejectJoin(r.Context(), quinielaID, membershipID, caller.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Leave handles DELETE /api/v1/groups/{id}/members/me.
//
// @Summary      Leave a group
// @Description  Removes the authenticated user from the group. Only the user
//
//	themselves may leave - no admin or owner can remove another
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	quinielaID, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if err := h.memberSvc.Leave(r.Context(), quinielaID, caller.ID); err != nil {
		writeError(w, r, h.log, err)
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	req, err := decodeJSON[renameGroupRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	quiniela, err := h.quinielaSvc.RenameGroup(r.Context(), id, caller.ID, req.Name)
	if err != nil {
		writeError(w, r, h.log, err)
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	members, err := h.memberSvc.ListByUser(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	out := make([]MyGroupResponse, 0, len(members))
	for _, m := range members {
		if m.Status == domain.MembershipLeft {
			continue // left/rejected memberships are hidden from the dashboard
		}
		out = append(out, MyGroupResponse{
			ID:                m.QuinielaID,
			Name:              m.GroupName,
			GroupStatus:       m.GroupStatus,
			MembershipStatus:  string(m.Status),
			Role:              string(m.Role),
			InviteCode:        m.InviteCode,
			EntryFee:          m.EntryFee,
			Currency:          m.Currency,
			IsPremium:         m.IsPremium,
			ModeGeneral:       m.ModeGeneral,
			ModeRound:         m.ModeRound,
			ActiveMemberCount: m.ActiveMemberCount,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GroupLivePredictionsResponse is the JSON envelope for GET /api/v1/groups/{id}/live-predictions.
type GroupLivePredictionsResponse struct {
	LiveMatches     []MatchResponse         `json:"live_matches"`
	UserPredictions []UserLivePredictionRow `json:"user_predictions"`
}

// UserLivePredictionRow holds one member's predictions for the currently-live matches.
type UserLivePredictionRow struct {
	UserID      int                       `json:"user_id"`
	DisplayName string                    `json:"display_name"`
	Predictions []MatchPredictionSnapshot `json:"predictions"`
}

// MatchPredictionSnapshot is a member's predicted score for a single live match.
// HasPrediction is false when the member has not submitted a prediction, in which
// case HomeScore and AwayScore are zero-valued.
type MatchPredictionSnapshot struct {
	MatchID       int  `json:"match_id"`
	HomeScore     int  `json:"home_score"`
	AwayScore     int  `json:"away_score"`
	HasPrediction bool `json:"has_prediction"`
}

// predKey indexes a prediction by (userID, matchID) for O(1) lookup.
type predKey struct{ userID, matchID int }

// buildSnapshots returns one MatchPredictionSnapshot per match ID, populated
// from predMap when a prediction exists for the given member.
func buildSnapshots(memberUserID int, matchIDs []int, predMap map[predKey]*domain.Prediction) []MatchPredictionSnapshot {
	snaps := make([]MatchPredictionSnapshot, len(matchIDs))
	for i, mID := range matchIDs {
		snap := MatchPredictionSnapshot{MatchID: mID}
		if p, ok := predMap[predKey{memberUserID, mID}]; ok {
			snap.HomeScore = p.HomeScore
			snap.AwayScore = p.AwayScore
			snap.HasPrediction = true
		}
		snaps[i] = snap
	}
	return snaps
}

// GetLivePredictions handles GET /api/v1/groups/{id}/live-predictions.
//
// @Summary      Live match predictions for all group members
// @Description  Returns the currently-live matches and each active member's predicted
//
//	score. Used by the live-predictions carousel on the tournament detail page.
//	Returns empty arrays when no matches are currently in progress.
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "Group ID"
// @Success      200  {object}  handler.GroupLivePredictionsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Not an active member of this group"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/live-predictions [get]
func (h *GroupHandler) GetLivePredictions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if _, ok := requireGroupMember(w, r, h.log, h.authz, id); !ok {
		return
	}

	ctx := r.Context()

	liveMatches, err := h.matchSvc.ListMatchesByStatus(ctx, domain.MatchStatusLive)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if len(liveMatches) == 0 {
		writeJSON(w, http.StatusOK, GroupLivePredictionsResponse{
			LiveMatches:     []MatchResponse{},
			UserPredictions: []UserLivePredictionRow{},
		})
		return
	}

	matchIDs := make([]int, len(liveMatches))
	for i, m := range liveMatches {
		matchIDs[i] = m.ID
	}

	predictions, err := h.predRepo.ListByGroupAndMatches(ctx, id, matchIDs)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	members, err := h.memberSvc.ListByQuiniela(ctx, id)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	predMap := make(map[predKey]*domain.Prediction, len(predictions))
	for _, p := range predictions {
		predMap[predKey{p.UserID, p.MatchID}] = p
	}

	matchResponses := make([]MatchResponse, len(liveMatches))
	for i, m := range liveMatches {
		matchResponses[i] = matchToResponse(m)
	}

	rows := make([]UserLivePredictionRow, 0, len(members))
	for _, m := range members {
		if m.Status != domain.MembershipActive {
			continue
		}
		rows = append(rows, UserLivePredictionRow{
			UserID:      m.UserID,
			DisplayName: m.DisplayName,
			Predictions: buildSnapshots(m.UserID, matchIDs, predMap),
		})
	}

	writeJSON(w, http.StatusOK, GroupLivePredictionsResponse{
		LiveMatches:     matchResponses,
		UserPredictions: rows,
	})
}

// setTournamentModeRequest is the JSON body for PATCH /api/v1/groups/{id}/tournament-mode.
type setTournamentModeRequest struct {
	ModeGeneral bool `json:"mode_general"`
	ModeRound   bool `json:"mode_round"`
}

// SetTournamentMode handles PATCH /api/v1/groups/{id}/tournament-mode.
//
// @Summary      Set tournament payment mode
// @Description  Configures the hybrid payment mode for a group. Only the group
//
//	owner may call this endpoint. Enabling any paid mode (mode_general or
//	mode_round) requires the group to have ≥ 5 active members. Disabling
//	both modes reverts the group to free (is_premium=false, no prizes).
//
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int                            true  "Group ID"
// @Param        body body      handler.setTournamentModeRequest  true  "Mode flags"
// @Success      200  {object}  handler.GroupResponse
// @Failure      400  {object}  handler.ErrorResponse  "Validation error"
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Not the group owner"
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      409  {object}  handler.ErrorResponse  "Not enough members to enable premium"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/tournament-mode [patch]
type updateRequireApprovalRequest struct {
	RequireApproval bool `json:"require_approval"`
}

// UpdateRequireApproval handles PATCH /api/v1/groups/{id}/require-approval.
// Only the group owner may call this. When require_approval is false, users
// who join via invite code are auto-activated with no pending step or notification.
func (h *GroupHandler) UpdateRequireApproval(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[updateRequireApprovalRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	q, err := h.quinielaSvc.UpdateRequireApproval(r.Context(), id, caller.ID, req.RequireApproval)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, groupToResponse(q))
}

type updateScoreFromZeroRequest struct {
	ScoreFromZero bool `json:"score_from_zero"`
}

// UpdateScoreFromZero handles PATCH /api/v1/groups/{id}/score-from-zero.
// Only the group owner may call this. When score_from_zero is true, members
// who join after this point accumulate points only for matches that kick off
// after their join date (they start at 0 within this group).
func (h *GroupHandler) UpdateScoreFromZero(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[updateScoreFromZeroRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	q, err := h.quinielaSvc.UpdateScoreFromZero(r.Context(), id, caller.ID, req.ScoreFromZero)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, groupToResponse(q))
}

// SetTournamentMode handles PATCH /api/v1/groups/{id}/tournament-mode.
func (h *GroupHandler) SetTournamentMode(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[setTournamentModeRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	q, err := h.quinielaSvc.SetTournamentMode(r.Context(), id, caller.ID, req.ModeGeneral, req.ModeRound)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, groupToResponse(q))
}
