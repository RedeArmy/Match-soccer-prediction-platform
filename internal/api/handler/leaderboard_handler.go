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
	authz  service.GroupAuthz
	log    *zap.Logger
}

// NewLeaderboardHandler constructs a LeaderboardHandler.
// authz enforces that only active group members may read standings.
func NewLeaderboardHandler(ranker service.Ranker, authz service.GroupAuthz, log *zap.Logger) *LeaderboardHandler {
	return &LeaderboardHandler{ranker: ranker, authz: authz, log: log}
}

// GetLeaderboard handles GET /api/v1/groups/{id}/leaderboard.
//
// An optional "phase" query parameter restricts the standings to predictions
// on matches belonging to a specific tournament phase (e.g. "group_stage").
// Omitting the parameter returns the overall standings across all phases.
//
// The response includes prize metadata (active_paid_members, winner_count,
// eligible_for_prizes) so clients can display prize positions and eligibility
// status without a separate API call.
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
// @Failure      403    {object}  handler.ErrorResponse  "Not an active member of this group"
// @Failure      404    {object}  handler.ErrorResponse
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/groups/{id}/leaderboard [get]
func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if _, ok := requireGroupMember(w, r, h.log, h.authz, id); !ok {
		return
	}

	phase := domain.MatchPhase(r.URL.Query().Get("phase"))
	if err := domain.ValidateMatchPhase(phase); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	breakdown := r.URL.Query().Get("breakdown") == "true"

	var result *service.LeaderboardResult
	switch {
	case breakdown:
		result, err = h.ranker.GetLeaderboardWithRoundBreakdown(r.Context(), id)
	case phase != "":
		result, err = h.ranker.GetPhaseLeaderboard(r.Context(), id, phase)
	default:
		result, err = h.ranker.GetLeaderboard(r.Context(), id)
	}
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	// Normalise nil result (group exists but has no scored predictions yet).
	if result == nil {
		result = &service.LeaderboardResult{}
	}

	// Return an empty array rather than null when there are no entries yet.
	out := make([]LeaderboardEntryResponse, 0, len(result.Entries))
	for _, e := range result.Entries {
		out = append(out, LeaderboardEntryResponse{
			Rank:        e.Rank,
			UserID:      e.User.ID,
			UserName:    userDisplayName(e.User),
			TotalPoints: e.TotalPoints,
			PrizeWinner: e.PrizeWinner,
			RoundPoints: e.RoundPoints,
		})
	}

	resp := LeaderboardResponse{
		QuinielaID:        id,
		Phase:             string(phase),
		ActivePaidMembers: result.ActivePaidMembers,
		WinnerCount:       result.WinnerCount,
		EligibleForPrizes: result.EligibleForPrizes,
		Entries:           out,
	}
	writeJSON(w, http.StatusOK, resp)
}
