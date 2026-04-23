package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const msgInvalidGroupID = "invalid group id"

// AdminGroupHandler handles admin endpoints for group management.
type AdminGroupHandler struct {
	svc service.AdminGroupService
	log *zap.Logger
}

// NewAdminGroupHandler constructs an AdminGroupHandler.
func NewAdminGroupHandler(svc service.AdminGroupService, log *zap.Logger) *AdminGroupHandler {
	return &AdminGroupHandler{svc: svc, log: log}
}

// DeleteGroup handles DELETE /admin/groups/{id}.
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

	var req updateGroupSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
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

	var req transferOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewOwnerUserID <= 0 {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	if err := h.svc.TransferOwnership(r.Context(), id, req.NewOwnerUserID, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
