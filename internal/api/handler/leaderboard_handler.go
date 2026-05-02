package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// LeaderboardHandler handles HTTP requests for group leaderboard endpoints.
type LeaderboardHandler struct {
	ranker service.Ranker
	log    *zap.Logger
}

// NewLeaderboardHandler constructs a LeaderboardHandler.
func NewLeaderboardHandler(ranker service.Ranker, log *zap.Logger) *LeaderboardHandler {
	return &LeaderboardHandler{ranker: ranker, log: log}
}

// GetLeaderboard handles GET /api/v1/groups/{id}/leaderboard.
//
// An optional "phase" query parameter restricts the standings to predictions
// on matches belonging to a specific tournament phase (e.g. "group_stage").
// Omitting the parameter returns the overall standings across all phases.
//
// @Summary      Group leaderboard
// @Description  Returns the ranked standings for a group. Pass ?phase=<value>
//
//	to restrict to a single tournament phase. Recognised phase values:
//	group_stage, round_of_32, round_of_16, quarter_final, semi_final,
//	third_place, final.
//
// @Tags         groups
// @Produce      json
// @Security     BearerAuth
// @Param        id     path      int     true   "Group ID"
// @Param        phase  query     string  false  "Tournament phase filter"
// @Success      200    {object}  handler.LeaderboardResponse
// @Failure      400    {object}  handler.ErrorResponse  "Unknown phase value"
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      404    {object}  handler.ErrorResponse
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/leaderboard [get]
func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	phase := domain.MatchPhase(r.URL.Query().Get("phase"))
	if err := domain.ValidateMatchPhase(phase); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	var entries []*domain.LeaderboardEntry
	if phase == "" {
		entries, err = h.ranker.GetLeaderboard(r.Context(), id)
	} else {
		entries, err = h.ranker.GetPhaseLeaderboard(r.Context(), id, phase)
	}
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	// Return an empty array rather than null when there are no entries yet.
	out := make([]LeaderboardEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, LeaderboardEntryResponse{
			Rank:        e.Rank,
			UserID:      e.User.ID,
			UserName:    e.User.Name,
			TotalPoints: e.TotalPoints,
			PrizeWinner: e.PrizeWinner,
		})
	}

	resp := LeaderboardResponse{
		QuinielaID: id,
		Phase:      string(phase),
		Entries:    out,
	}
	writeJSON(w, http.StatusOK, resp)
}
