package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminLeaderboardHandler handles admin endpoints for leaderboard and prediction data.
type AdminLeaderboardHandler struct {
	svc    service.AdminReadService
	params service.SystemParamService
	log    *zap.Logger
}

// NewAdminLeaderboardHandler constructs an AdminLeaderboardHandler.
func NewAdminLeaderboardHandler(svc service.AdminReadService, params service.SystemParamService, log *zap.Logger) *AdminLeaderboardHandler {
	return &AdminLeaderboardHandler{svc: svc, params: params, log: log}
}

// GlobalLeaderboard handles GET /admin/leaderboard - top N users by total points across all groups.
//
// @Summary      Global leaderboard
// @Description  Returns the top N users ranked by total scored points across all
//
//	quiniela groups. Default and maximum limits are read from system_params
//	(pagination.default_limit / pagination.max_limit). Requires admin role.
//
// @Tags         admin-leaderboard
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query     int  false  "Max entries to return (default/max from system_params)"
// @Success      200    {array}   handler.GlobalLeaderboardEntryResponse
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/admin/leaderboard [get]
func (h *AdminLeaderboardHandler) GlobalLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	defaultLimit := h.params.GetInt(ctx, domain.ParamKeyPaginationDefaultLimit, domain.DefaultPaginationDefaultLimit)
	maxLimit := h.params.GetInt(ctx, domain.ParamKeyPaginationMaxLimit, domain.DefaultPaginationMaxLimit)
	limit := defaultLimit
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= maxLimit {
		limit = l
	}

	entries, err := h.svc.GlobalLeaderboard(r.Context(), limit)
	if err != nil {
		writeError(w, r, h.log, err)
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
//
// @Summary      Group leaderboard snapshot history
// @Description  Returns the most recent point-in-time leaderboard snapshots for
//
//	the given group. Snapshots are taken automatically by the scoring
//	worker after each match is scored. Default and maximum limits are read
//	from system_params (pagination.default_limit / pagination.max_limit).
//	Requires admin role.
//
// @Tags         admin-groups
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int  true   "Group ID"
// @Param        limit  query     int  false  "Max snapshots to return (default/max from system_params)"
// @Success      200    {array}   handler.SnapshotResponse
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422    {object}  handler.ErrorResponse  "Invalid group ID"
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/leaderboard/history [get]
func (h *AdminLeaderboardHandler) SnapshotHistory(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || groupID <= 0 {
		writeError(w, r, h.log, apperrors.Validation("invalid group id"))
		return
	}

	ctx := r.Context()
	defaultLimit := h.params.GetInt(ctx, domain.ParamKeyPaginationDefaultLimit, domain.DefaultPaginationDefaultLimit)
	maxLimit := h.params.GetInt(ctx, domain.ParamKeyPaginationMaxLimit, domain.DefaultPaginationMaxLimit)
	limit := defaultLimit
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= maxLimit {
		limit = l
	}

	snapshots, err := h.svc.ListSnapshotHistory(r.Context(), groupID, limit)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]SnapshotResponse, len(snapshots))
	for i, s := range snapshots {
		data[i] = snapshotToResponse(s)
	}
	writeJSON(w, http.StatusOK, data)
}

// ListPredictions handles GET /admin/predictions - all predictions with optional filters.
//
// @Summary      List all predictions
// @Description  Returns a paginated list of predictions across all users and groups.
//
//	Supports filtering by user, match, and group. Requires admin role.
//
// @Tags         admin-leaderboard
// @Produce      json
// @Security     BearerAuth
// @Param        user_id     query     int  false  "Filter by user ID"
// @Param        match_id    query     int  false  "Filter by match ID"
// @Param        quiniela_id query     int  false  "Filter by group ID"
// @Param        limit       query     int  false  "Max records per page (default 50, max 200)"
// @Param        page        query     int  false  "Page number (default 1)"
// @Success      200         {object}  handler.Paged[handler.PredictionResponse]
// @Failure      401         {object}  handler.ErrorResponse
// @Failure      403         {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500         {object}  handler.ErrorResponse
// @Router       /api/v1/admin/predictions [get]
func (h *AdminLeaderboardHandler) ListPredictions(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	f := repository.PredictionAdminFilters{
		UserID:     parseOptionalInt(r, "user_id"),
		MatchID:    parseOptionalInt(r, "match_id"),
		QuinielaID: parseOptionalInt(r, "quiniela_id"),
	}

	predictions, err := h.svc.ListPredictions(r.Context(), f, p)
	if err != nil {
		writeError(w, r, h.log, err)
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
//
// @Summary      List predictions by match
// @Description  Returns all predictions for a specific match across all users and
//
//	groups, paginated. Useful for reviewing scoring after a match
//	result is set. Requires admin role.
//
// @Tags         admin-leaderboard
// @Produce      json
// @Security     BearerAuth
// @Param        matchID  path      int  true   "Match ID"
// @Param        limit    query     int  false  "Max records per page (default 50, max 200)"
// @Param        page     query     int  false  "Page number (default 1)"
// @Success      200      {array}   handler.PredictionResponse
// @Failure      401      {object}  handler.ErrorResponse
// @Failure      403      {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422      {object}  handler.ErrorResponse  "Invalid match ID"
// @Failure      500      {object}  handler.ErrorResponse
// @Router       /api/v1/admin/predictions/match/{matchID} [get]
func (h *AdminLeaderboardHandler) ListPredictionsByMatch(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.Atoi(chi.URLParam(r, "matchID"))
	if err != nil || matchID <= 0 {
		writeError(w, r, h.log, apperrors.Validation("invalid match id"))
		return
	}

	p := parsePagination(r)
	f := repository.PredictionAdminFilters{MatchID: &matchID}

	predictions, err := h.svc.ListPredictions(r.Context(), f, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]PredictionResponse, len(predictions))
	for i, pred := range predictions {
		data[i] = predToResponse(pred)
	}
	writeJSON(w, http.StatusOK, data)
}
