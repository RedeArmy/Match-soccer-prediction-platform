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
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	leaderboardAlice      = "Alice"
	leaderboardPath       = "/groups/1/leaderboard"
	leaderboardPhasePath  = "/groups/1/leaderboard?phase=group_stage"
	leaderboardCallerID   = 99
	publicLeaderboardPath = "/api/public/groups/leaderboard?code=TEST"
)

// buildPublicLeaderboardRouter wires a PublicGroupHandler with the given entries
// so userDisplayName fallback branches can be exercised via HTTP.
func buildPublicLeaderboardRouter(entries []*domain.LeaderboardEntry) http.Handler {
	resolver := &stubCodeResolver{q: &domain.Quiniela{ID: 1, InviteCode: "TEST"}}
	ranker := &stubPublicRanker{result: &service.LeaderboardResult{Entries: entries}}
	return newPublicGroupRouter(resolver, ranker)
}

// routeLeaderboard wires a LeaderboardHandler into a chi router for testing.
// A default caller (ID = leaderboardCallerID) is injected into every request
// via middleware, mirroring the ResolveUser middleware present in production.
// Tests that exercise the unauthenticated path should build the handler
// directly without this helper.
func routeLeaderboard(t *testing.T, ranker *stubRanker, authz *stubGroupAuthz) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := middleware.ContextWithUser(r.Context(), &domain.User{ID: leaderboardCallerID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	h := handler.NewLeaderboardHandler(ranker, authz, zaptest.NewLogger(t))
	r.Get("/groups/{id}/leaderboard", h.GetLeaderboard)
	return r
}

// ── GetLeaderboard ────────────────────────────────────────────────────────────

func TestGetLeaderboard_EmptyGroup_Returns200WithEmptyArray(t *testing.T) {
	ranker := &stubRanker{entries: nil}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

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
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

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
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

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
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for unknown phase, got %d", w.Code)
	}
}

func TestGetLeaderboard_ServiceError_Returns500(t *testing.T) {
	ranker := &stubRanker{err: apperrors.Internal(errors.New("db down"))}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, w.Code)
	}
}

func TestGetLeaderboard_NotFound_Returns404(t *testing.T) {
	ranker := &stubRanker{err: apperrors.NotFound("quiniela not found")}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf(fmtExpect404, w.Code)
	}
}

func TestGetLeaderboard_QuinielaIDInResponse(t *testing.T) {
	ranker := &stubRanker{entries: nil}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

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
	routeLeaderboard(t, ranker, &stubGroupAuthz{}).ServeHTTP(w, req)

	var resp handler.LeaderboardResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if resp.Phase != "" {
		t.Errorf("expected phase to be empty for overall leaderboard, got %q", resp.Phase)
	}
}

func TestGetLeaderboard_NonMember_Returns403(t *testing.T) {
	ranker := &stubRanker{}
	authz := &stubGroupAuthz{memberErr: apperrors.Forbidden("caller is not an active member of this group")}
	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	routeLeaderboard(t, ranker, authz).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-member, got %d", w.Code)
	}
}

func TestGetLeaderboard_Unauthenticated_Returns401(t *testing.T) {
	ranker := &stubRanker{}
	// Build the handler directly — no user-injection middleware — to simulate
	// a request that bypassed RequireAuth (should never happen in production,
	// but the handler must still guard against it).
	r := chi.NewRouter()
	h := handler.NewLeaderboardHandler(ranker, &stubGroupAuthz{}, zaptest.NewLogger(t))
	r.Get("/groups/{id}/leaderboard", h.GetLeaderboard)

	req := httptest.NewRequest(http.MethodGet, leaderboardPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no user in context, got %d", w.Code)
	}
}

// ── userDisplayName ───────────────────────────────────────────────────────────
// userDisplayName is tested via the public leaderboard endpoint which calls it
// in GetPublicLeaderboard. Direct unit tests cover the three fallback branches.

func TestUserDisplayName_UsesUsername_WhenSet(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Username: "jugador99", Name: "Full Name", Email: "u@example.com"}, Rank: 1},
	}
	r := buildPublicLeaderboardRouter(entries)
	req := httptest.NewRequest(http.MethodGet, publicLeaderboardPath, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp handler.PublicLeaderboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].UserName != "jugador99" {
		t.Errorf("expected display name 'jugador99', got %q", resp.Entries[0].UserName)
	}
}

func TestUserDisplayName_FallsBackToName_WhenUsernameEmpty(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1, Username: "", Name: "Ana García", Email: "ana@example.com"}, Rank: 1},
	}
	r := buildPublicLeaderboardRouter(entries)
	req := httptest.NewRequest(http.MethodGet, publicLeaderboardPath, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp handler.PublicLeaderboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].UserName != "Ana García" {
		t.Errorf("expected display name 'Ana García', got %q", resp.Entries[0].UserName)
	}
}

func TestUserDisplayName_FallsBackToEmail_WhenUsernameAndNameEmpty(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 2, Username: "", Name: "", Email: "bob@example.com"}, Rank: 1},
	}
	r := buildPublicLeaderboardRouter(entries)
	req := httptest.NewRequest(http.MethodGet, publicLeaderboardPath, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp handler.PublicLeaderboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(resp.Entries) == 0 || resp.Entries[0].UserName != "bob@example.com" {
		t.Errorf("expected display name 'bob@example.com', got %q", resp.Entries[0].UserName)
	}
}
