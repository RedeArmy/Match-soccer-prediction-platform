// Package handler contains the HTTP request handlers for the World Cup
// quiniela REST API.
//
// Each handler file in this package is responsible for a single resource:
// parsing the HTTP request, delegating to the appropriate service, and
// translating the result (or error) into an HTTP response. Handlers must
// contain no business logic; they are a thin translation layer between
// the HTTP protocol and the service layer.
//
// Dependencies (services, logger) are injected via the constructor of the
// enclosing struct rather than through package-level globals, which keeps
// handlers testable in isolation using httptest.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// MatchHandler handles HTTP requests for the /api/v1/matches resource.
type MatchHandler struct {
	svc service.MatchService
	log *zap.Logger
}

// NewMatchHandler constructs a MatchHandler.
func NewMatchHandler(svc service.MatchService, log *zap.Logger) *MatchHandler {
	return &MatchHandler{svc: svc, log: log}
}

// createMatchRequest is the JSON body accepted by POST /api/v1/matches.
type createMatchRequest struct {
	HomeTeam  string    `json:"home_team"`
	AwayTeam  string    `json:"away_team"`
	KickoffAt time.Time `json:"kickoff_at"`
}

// updateResultRequest is the JSON body accepted by PATCH /api/v1/matches/{id}.
type updateResultRequest struct {
	HomeScore *int `json:"home_score"`
	AwayScore *int `json:"away_score"`
}

// ListMatches handles GET /api/v1/matches.
//
// @Summary      List matches
// @Description  Returns all fixtures in the tournament schedule.
// @Tags         matches
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.MatchResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/matches [get]
func (h *MatchHandler) ListMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := h.svc.ListMatches(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	out := make([]MatchResponse, len(matches))
	for i, m := range matches {
		out[i] = matchToResponse(m)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetMatch handles GET /api/v1/matches/{id}.
//
// @Summary      Get a match
// @Description  Returns a single fixture by ID.
// @Tags         matches
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Match ID"
// @Success      200  {object}  handler.MatchResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/matches/{id} [get]
func (h *MatchHandler) GetMatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	match, err := h.svc.GetMatch(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, matchToResponse(match))
}

// CreateMatch handles POST /api/v1/matches (admin only).
//
// @Summary      Create a match
// @Description  Admin only. Creates a new fixture in the tournament schedule.
// @Tags         matches
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.createMatchRequest  true  "Match to create"
// @Success      201   {object}  handler.MatchResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/matches [post]
func (h *MatchHandler) CreateMatch(w http.ResponseWriter, r *http.Request) {
	var req createMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}
	match := &domain.Match{
		HomeTeam:  req.HomeTeam,
		AwayTeam:  req.AwayTeam,
		KickoffAt: req.KickoffAt,
	}
	if err := h.svc.CreateMatch(r.Context(), match); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, matchToResponse(match))
}

// UpdateResult handles PATCH /api/v1/matches/{id} (admin only).
//
// @Summary      Update match result
// @Description  Admin only. Sets the final score for a finished match.
// @Tags         matches
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          true  "Match ID"
// @Param        body  body      handler.updateResultRequest  true  "Final score"
// @Success      200   {object}  handler.MatchResponse
// @Failure      404   {object}  handler.ErrorResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/matches/{id} [patch]
func (h *MatchHandler) UpdateResult(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	var req updateResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}
	if req.HomeScore == nil || req.AwayScore == nil {
		middleware.WriteError(w, r, h.log, decodeError(nil))
		return
	}
	match, err := h.svc.UpdateResult(r.Context(), id, *req.HomeScore, *req.AwayScore)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, matchToResponse(match))
}

// StartMatch handles POST /api/v1/matches/{id}/start (admin only).
//
// @Summary      Start a match
// @Description  Admin only. Transitions a match from scheduled to live.
// @Tags         matches
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Match ID"
// @Success      200  {object}  handler.MatchResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      422  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/matches/{id}/start [post]
func (h *MatchHandler) StartMatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	match, err := h.svc.StartMatch(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, matchToResponse(match))
}

// pathID extracts a numeric path parameter from the chi URL context.
func pathID(r *http.Request, param string) (int, error) {
	raw := chi.URLParam(r, param)
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, decodeError(err)
	}
	return id, nil
}
