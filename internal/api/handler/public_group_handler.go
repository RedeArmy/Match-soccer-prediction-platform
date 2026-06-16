package handler

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// publicCodeResolver is the narrow read-only interface the handler needs to
// resolve an invite code without loading a full authenticated session.
type publicCodeResolver interface {
	GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error)
}

// PublicGroupHandler serves unauthenticated group preview endpoints.
// Only the leaderboard classification is exposed — no join, no prize pool,
// no group type information is returned.
type PublicGroupHandler struct {
	quinielas publicCodeResolver
	ranker    service.Ranker
	log       *zap.Logger
}

// NewPublicGroupHandler constructs a PublicGroupHandler.
func NewPublicGroupHandler(quinielas publicCodeResolver, ranker service.Ranker, log *zap.Logger) *PublicGroupHandler {
	return &PublicGroupHandler{quinielas: quinielas, ranker: ranker, log: log}
}

// GetPublicLeaderboard handles GET /api/public/groups/leaderboard?code=XXXXX.
// No authentication is required. Returns the group name and the ranked standings
// with per-round breakdowns. Prize-winner flags are stripped to prevent exposing
// internal payment state to unauthenticated callers.
func (h *PublicGroupHandler) GetPublicLeaderboard(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, r, h.log, apperrors.Validation("code is required"))
		return
	}

	q, err := h.quinielas.GetByInviteCode(r.Context(), code)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	result, err := h.ranker.GetLeaderboardWithRoundBreakdown(r.Context(), q.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if result == nil {
		result = &service.LeaderboardResult{}
	}

	entries := make([]LeaderboardEntryResponse, 0, len(result.Entries))
	for _, e := range result.Entries {
		entries = append(entries, LeaderboardEntryResponse{
			Rank:        e.Rank,
			UserID:      e.User.ID,
			UserName:    userDisplayName(e.User),
			TotalPoints: e.TotalPoints,
			PrizeWinner: false, // deliberately omitted on the public endpoint
			RoundPoints: e.RoundPoints,
		})
	}

	writeJSON(w, http.StatusOK, PublicLeaderboardResponse{
		GroupName: q.Name,
		Entries:   entries,
	})
}

// PublicLeaderboardResponse is the JSON body for GET /api/public/groups/leaderboard.
type PublicLeaderboardResponse struct {
	GroupName string                     `json:"group_name"`
	Entries   []LeaderboardEntryResponse `json:"entries"`
}
