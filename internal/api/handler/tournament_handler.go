package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// TournamentHandler handles HTTP requests for the tournament sub-resources:
// real-time group standings and admin-managed bracket slots.
type TournamentHandler struct {
	svc service.TournamentService
	log *zap.Logger
}

// NewTournamentHandler constructs a TournamentHandler.
func NewTournamentHandler(svc service.TournamentService, log *zap.Logger) *TournamentHandler {
	return &TournamentHandler{svc: svc, log: log}
}

type createSlotRequest struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type confirmSlotRequest struct {
	Team string `json:"team"`
}

// GetAllStandings handles GET /api/v1/tournament/standings.
// Returns real-time group standings for all groups.
func (h *TournamentHandler) GetAllStandings(w http.ResponseWriter, r *http.Request) {
	standings, err := h.svc.GetAllStandings(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, allStandingsToResponse(standings))
}

// GetGroupStanding handles GET /api/v1/tournament/standings/{group}.
// Returns real-time standings for the requested group (e.g. "A").
func (h *TournamentHandler) GetGroupStanding(w http.ResponseWriter, r *http.Request) {
	group := chi.URLParam(r, "group")
	entries, err := h.svc.GetGroupStanding(r.Context(), group)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	rows := make([]GroupStandingResponse, len(entries))
	for i, e := range entries {
		rows[i] = standingToResponse(e)
	}
	writeJSON(w, http.StatusOK, rows)
}

// ListTeams handles GET /api/v1/teams.
// Returns all team names sorted A → Z. Public — no authentication required.
func (h *TournamentHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	names, err := h.svc.ListTeamNames(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// ListSlots handles GET /api/v1/tournament/slots.
// Returns all bracket position slots.
func (h *TournamentHandler) ListSlots(w http.ResponseWriter, r *http.Request) {
	slots, err := h.svc.ListSlots(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]TournamentSlotResponse, len(slots))
	for i, s := range slots {
		resp[i] = slotToResponse(s)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateSlot handles POST /api/v1/tournament/slots.
// Only the system administrator may call this (enforced by RequireRole middleware).
func (h *TournamentHandler) CreateSlot(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[createSlotRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	slot, err := h.svc.CreateSlot(r.Context(), req.Label, req.Description)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, slotToResponse(slot))
}

// BestThirdAssignmentResponse is the JSON shape for one best-3rd slot assignment.
type BestThirdAssignmentResponse struct {
	SlotLabel string `json:"slot_label"`
	Group     string `json:"group"`
	Team      string `json:"team"`
}

// ConfirmBestThirds handles POST /api/v1/tournament/slots/confirm-best-thirds.
// Ranks the 12 group-stage third-placed teams, picks the best 8, resolves the
// correct r32 slot for each via bipartite matching, and confirms them in bulk.
// Returns Validation (400) when the group stage is not yet fully complete.
// Only the system administrator may call this.
func (h *TournamentHandler) ConfirmBestThirds(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	assignments, err := h.svc.AutoConfirmBestThirdSlots(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	resp := make([]BestThirdAssignmentResponse, len(assignments))
	for i, a := range assignments {
		resp[i] = BestThirdAssignmentResponse{SlotLabel: a.SlotLabel, Group: a.Group, Team: a.Team}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ConfirmSlot handles PATCH /api/v1/tournament/slots/{id}.
// Only the system administrator may call this (enforced by RequireRole middleware).
func (h *TournamentHandler) ConfirmSlot(w http.ResponseWriter, r *http.Request) {
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

	req, err := decodeJSON[confirmSlotRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	slot, err := h.svc.ConfirmSlot(r.Context(), id, caller.ID, req.Team)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, slotToResponse(slot))
}
