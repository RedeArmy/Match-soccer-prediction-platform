package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminLeaderboardHandler handles admin endpoints for leaderboard and prediction data.
type AdminLeaderboardHandler struct {
	svc service.AdminReadService
	log *zap.Logger
}

// NewAdminLeaderboardHandler constructs an AdminLeaderboardHandler.
func NewAdminLeaderboardHandler(svc service.AdminReadService, log *zap.Logger) *AdminLeaderboardHandler {
	return &AdminLeaderboardHandler{svc: svc, log: log}
}

// GlobalLeaderboard handles GET /admin/leaderboard — top N users by total points across all groups.
func (h *AdminLeaderboardHandler) GlobalLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	entries, err := h.svc.GlobalLeaderboard(r.Context(), limit)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]GlobalLeaderboardEntryResponse, len(entries))
	for i, e := range entries {
		data[i] = GlobalLeaderboardEntryResponse{
			Rank:        e.Rank,
			UserID:      e.UserID,
			UserName:    e.UserName,
			TotalPoints: e.TotalPoints,
		}
	}
	writeJSON(w, http.StatusOK, data)
}

// SnapshotHistory handles GET /admin/groups/{id}/leaderboard/history.
func (h *AdminLeaderboardHandler) SnapshotHistory(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || groupID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid group id"))
		return
	}

	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	snapshots, err := h.svc.ListSnapshotHistory(r.Context(), groupID, limit)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]SnapshotResponse, len(snapshots))
	for i, s := range snapshots {
		data[i] = snapshotToResponse(s)
	}
	writeJSON(w, http.StatusOK, data)
}

// ListPredictions handles GET /admin/predictions — all predictions with optional filters.
func (h *AdminLeaderboardHandler) ListPredictions(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	f := repository.PredictionAdminFilters{
		UserID:     parseOptionalInt(r, "user_id"),
		MatchID:    parseOptionalInt(r, "match_id"),
		QuinielaID: parseOptionalInt(r, "quiniela_id"),
	}

	predictions, err := h.svc.ListPredictions(r.Context(), f, p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]PredictionResponse, len(predictions))
	for i, pred := range predictions {
		data[i] = predToResponse(pred)
	}
	writeJSON(w, http.StatusOK, Paged[PredictionResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}

// ListPredictionsByMatch handles GET /admin/predictions/match/{matchID}.
func (h *AdminLeaderboardHandler) ListPredictionsByMatch(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.Atoi(chi.URLParam(r, "matchID"))
	if err != nil || matchID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid match id"))
		return
	}

	p := parsePagination(r)
	f := repository.PredictionAdminFilters{MatchID: &matchID}

	predictions, err := h.svc.ListPredictions(r.Context(), f, p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]PredictionResponse, len(predictions))
	for i, pred := range predictions {
		data[i] = predToResponse(pred)
	}
	writeJSON(w, http.StatusOK, data)
}
