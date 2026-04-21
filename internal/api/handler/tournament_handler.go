package handler

import (
	"encoding/json"
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
	Label string `json:"label"`
}

type confirmSlotRequest struct {
	Team string `json:"team"`
}

// GetAllStandings handles GET /api/v1/tournament/standings.
// Returns real-time group standings for all groups.
func (h *TournamentHandler) GetAllStandings(w http.ResponseWriter, r *http.Request) {
	standings, err := h.svc.GetAllStandings(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
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
		middleware.WriteError(w, r, h.log, err)
		return
	}
	rows := make([]GroupStandingResponse, len(entries))
	for i, e := range entries {
		rows[i] = standingToResponse(e)
	}
	writeJSON(w, http.StatusOK, rows)
}

// ListSlots handles GET /api/v1/tournament/slots.
// Returns all bracket position slots.
func (h *TournamentHandler) ListSlots(w http.ResponseWriter, r *http.Request) {
	slots, err := h.svc.ListSlots(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
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
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req createSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	slot, err := h.svc.CreateSlot(r.Context(), req.Label)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, slotToResponse(slot))
}

// ConfirmSlot handles PATCH /api/v1/tournament/slots/{id}.
// Only the system administrator may call this (enforced by RequireRole middleware).
func (h *TournamentHandler) ConfirmSlot(w http.ResponseWriter, r *http.Request) {
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

	var req confirmSlotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	slot, err := h.svc.ConfirmSlot(r.Context(), id, caller.ID, req.Team)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, slotToResponse(slot))
}
