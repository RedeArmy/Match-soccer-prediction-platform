package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	leaderboardAlice     = "Alice"
	leaderboardPath      = "/groups/1/leaderboard"
	leaderboardPhasePath = "/groups/1/leaderboard?phase=group_stage"
)

// routeLeaderboard wires a LeaderboardHandler into a chi router for testing.
func routeLeaderboard(t *testing.T, ranker *stubRanker) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	h := handler.NewLeaderboardHandler(ranker, zaptest.NewLogger(t))
	r.Get("/groups/{id}/leaderboard", h.GetLeaderboard)
	return r
}

// ── GetLeaderboard ────────────────────────────────────────────────────────────

func TestGetLeaderboard_EmptyGroup_Returns200WithEmptyArray(t *testing.T) {
	ranker := &stubRanker{entries: nil}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Entries == nil {
		t.Error("entries should be an empty array, not null")
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(resp.Entries))
	}
}

func TestGetLeaderboard_WithEntries_Returns200WithRankedList(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 2, Name: "Bob"}, TotalPoints: 25, Rank: 1, PrizeWinner: true},
		{User: &domain.User{ID: 1, Name: leaderboardAlice}, TotalPoints: 10, Rank: 2, PrizeWinner: false},
	}
	ranker := &stubRanker{entries: entries}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}
	if resp.Entries[0].UserName != "Bob" || !resp.Entries[0].PrizeWinner {
		t.Errorf("entry[0]: want Bob/prize_winner=true, got %s/%v", resp.Entries[0].UserName, resp.Entries[0].PrizeWinner)
	}
	if resp.Entries[1].UserName != leaderboardAlice || resp.Entries[1].PrizeWinner {
		t.Errorf("entry[1]: want Alice/prize_winner=false, got %s/%v", resp.Entries[1].UserName, resp.Entries[1].PrizeWinner)
	}
}

func TestGetLeaderboard_WithPhaseParam_Returns200(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Name: leaderboardAlice}, TotalPoints: 5, Rank: 1, PrizeWinner: true},
	}
	ranker := &stubRanker{entries: entries}
	req := httptest.NewRequest(http.MethodGet, leaderboardPhasePath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Phase != "group_stage" {
		t.Errorf("expected phase=group_stage, got %q", resp.Phase)
	}
}

func TestGetLeaderboard_UnknownPhase_Returns422(t *testing.T) {
	ranker := &stubRanker{}
	req := httptest.NewRequest(http.MethodGet, "/groups/1/leaderboard?phase=unknown_phase", nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for unknown phase, got %d", w.Code)
	}
}

func TestGetLeaderboard_ServiceError_Returns500(t *testing.T) {
	ranker := &stubRanker{err: apperrors.Internal(errors.New("db down"))}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestGetLeaderboard_NotFound_Returns404(t *testing.T) {
	ranker := &stubRanker{err: apperrors.NotFound("quiniela not found")}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf(fmtExpect404, w.Code)
	}
}

func TestGetLeaderboard_QuinielaIDInResponse(t *testing.T) {
	ranker := &stubRanker{entries: nil}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.QuinielaID != 1 {
		t.Errorf("expected quiniela_id=1, got %d", resp.QuinielaID)
	}
}

func TestGetLeaderboard_NoPhaseParam_PhaseOmittedFromResponse(t *testing.T) {
	ranker := &stubRanker{entries: nil}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker).ServeHTTP(w, req)

	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Phase != "" {
		t.Errorf("expected phase to be empty for overall leaderboard, got %q", resp.Phase)
	}
}
