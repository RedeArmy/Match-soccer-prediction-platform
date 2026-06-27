package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubCodeResolver struct {
	q   *domain.Quiniela
	err error
}

func (s *stubCodeResolver) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return s.q, s.err
}

type stubPublicRanker struct {
	result *service.LeaderboardResult
	err    error
}

func (s *stubPublicRanker) GetLeaderboard(_ context.Context, _ int) (*service.LeaderboardResult, error) {
	return s.result, s.err
}
func (s *stubPublicRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*service.LeaderboardResult, error) {
	return s.result, s.err
}
func (s *stubPublicRanker) GetLeaderboardWithRoundBreakdown(_ context.Context, _ int) (*service.LeaderboardResult, error) {
	return s.result, s.err
}

// ── router helper ─────────────────────────────────────────────────────────────

func newPublicGroupRouter(resolver *stubCodeResolver, ranker *stubPublicRanker) http.Handler {
	h := handler.NewPublicGroupHandler(resolver, ranker, zap.NewNop())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/public/groups/leaderboard", h.GetPublicLeaderboard)
	return mux
}

// ── GetPublicLeaderboard ──────────────────────────────────────────────────────

func TestGetPublicLeaderboard_MissingCode_Returns422(t *testing.T) {
	router := newPublicGroupRouter(&stubCodeResolver{}, &stubPublicRanker{})
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard", "")

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPublicLeaderboard_CodeNotFound_Returns404(t *testing.T) {
	resolver := &stubCodeResolver{err: apperrors.NotFound("group not found")}
	router := newPublicGroupRouter(resolver, &stubPublicRanker{})
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard?code=XXXX", "")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPublicLeaderboard_RankerError_Returns500(t *testing.T) {
	resolver := &stubCodeResolver{q: &domain.Quiniela{ID: 1, Name: "Mi Quiniela"}}
	ranker := &stubPublicRanker{err: errors.New("db timeout")}
	router := newPublicGroupRouter(resolver, ranker)
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard?code=ABC123", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetPublicLeaderboard_Success_Returns200WithEntries(t *testing.T) {
	resolver := &stubCodeResolver{q: &domain.Quiniela{ID: 7, Name: "Copa Familia"}}
	ranker := &stubPublicRanker{
		result: &service.LeaderboardResult{
			Entries: []*domain.LeaderboardEntry{
				{User: &domain.User{ID: 1, Name: "Jugador1"}, TotalPoints: 30, Rank: 1, PrizeWinner: true},
				{User: &domain.User{ID: 2, Name: "Jugador2"}, TotalPoints: 20, Rank: 2, PrizeWinner: false},
			},
		},
	}
	router := newPublicGroupRouter(resolver, ranker)
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard?code=FAM001", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp handler.PublicLeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.GroupName != "Copa Familia" {
		t.Errorf("group_name: want Copa Familia, got %s", resp.GroupName)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries: want 2, got %d", len(resp.Entries))
	}
	if resp.Entries[0].UserName != "Jugador1" {
		t.Errorf("entry[0].user_name: want Jugador1, got %s", resp.Entries[0].UserName)
	}
	if resp.Entries[0].TotalPoints != 30 {
		t.Errorf("entry[0].total_points: want 30, got %d", resp.Entries[0].TotalPoints)
	}
	// prize_winner and user_id are not included in the public response struct.
}

func TestGetPublicLeaderboard_NilResult_Returns200WithEmptyEntries(t *testing.T) {
	resolver := &stubCodeResolver{q: &domain.Quiniela{ID: 3, Name: "Vacío"}}
	ranker := &stubPublicRanker{result: nil}
	router := newPublicGroupRouter(resolver, ranker)
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard?code=EMPTY1", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp handler.PublicLeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(resp.Entries))
	}
}

func TestGetPublicLeaderboard_WhitespaceOnlyCode_Returns422(t *testing.T) {
	router := newPublicGroupRouter(&stubCodeResolver{}, &stubPublicRanker{})
	w := do(router, http.MethodGet, "/api/public/groups/leaderboard?code=+++", "")

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for whitespace-only code, got %d", w.Code)
	}
}
