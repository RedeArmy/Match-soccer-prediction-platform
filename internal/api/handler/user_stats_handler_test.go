package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

const pathUserStats = "/users/me/stats"

func testUserStatsRouter(h *handler.UserStatsHandler, user *domain.User) http.Handler {
	r := chi.NewRouter()
	if user != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(middleware.ContextWithUser(req.Context(), user)))
			})
		})
	}
	r.Get("/users/me/stats", h.GetMyStats)
	return r
}

func TestUserStatsHandler_GetMyStats_401_WhenNoUser(t *testing.T) {
	svc := &stubUserStatsSvc{stats: &domain.UserStats{}}
	h := handler.NewUserStatsHandler(svc, zaptest.NewLogger(t))
	router := testUserStatsRouter(h, nil)

	req := httptest.NewRequest(http.MethodGet, pathUserStats, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf(fmtExpect401, rr.Code)
	}
}

func TestUserStatsHandler_GetMyStats_500_WhenServiceFails(t *testing.T) {
	svc := &stubUserStatsSvc{err: errors.New("service error")}
	h := handler.NewUserStatsHandler(svc, zaptest.NewLogger(t))
	router := testUserStatsRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodGet, pathUserStats, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf(fmtExpect500, rr.Code)
	}
}

func TestUserStatsHandler_GetMyStats_200_ReturnsStats(t *testing.T) {
	groupPoints := map[domain.MatchPhase]int{
		domain.PhaseGroupStage:   10,
		domain.PhaseQuarterFinal: 6,
	}
	svc := &stubUserStatsSvc{
		stats: &domain.UserStats{
			TotalPredictions:   10,
			ScoredPredictions:  8,
			CorrectPredictions: 5,
			ExactPredictions:   2,
			TotalPoints:        16,
			PointsByPhase:      groupPoints,
			AccuracyPct:        62.5,
			AvgPointsPerPred:   2.0,
			CurrentStreak:      3,
			LongestStreak:      5,
		},
	}
	h := handler.NewUserStatsHandler(svc, zaptest.NewLogger(t))
	router := testUserStatsRouter(h, &domain.User{ID: 7})

	req := httptest.NewRequest(http.MethodGet, pathUserStats, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	if ct := rr.Header().Get(headerContentType); !strings.HasPrefix(ct, contentTypeJSON) {
		t.Errorf("Content-Type: want %s, got %s", contentTypeJSON, ct)
	}

	var body handler.UserStatsResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if body.TotalPredictions != 10 {
		t.Errorf("TotalPredictions: want 10, got %d", body.TotalPredictions)
	}
	if body.ScoredPredictions != 8 {
		t.Errorf("ScoredPredictions: want 8, got %d", body.ScoredPredictions)
	}
	if body.CorrectPredictions != 5 {
		t.Errorf("CorrectPredictions: want 5, got %d", body.CorrectPredictions)
	}
	if body.ExactPredictions != 2 {
		t.Errorf("ExactPredictions: want 2, got %d", body.ExactPredictions)
	}
	if body.TotalPoints != 16 {
		t.Errorf("TotalPoints: want 16, got %d", body.TotalPoints)
	}
	if body.AccuracyPct != 62.5 {
		t.Errorf("AccuracyPct: want 62.5, got %f", body.AccuracyPct)
	}
	if body.AvgPointsPerPred != 2.0 {
		t.Errorf("AvgPointsPerPred: want 2.0, got %f", body.AvgPointsPerPred)
	}
	if body.CurrentStreak != 3 {
		t.Errorf("CurrentStreak: want 3, got %d", body.CurrentStreak)
	}
	if body.LongestStreak != 5 {
		t.Errorf("LongestStreak: want 5, got %d", body.LongestStreak)
	}
	if len(body.PointsByPhase) != 2 {
		t.Errorf("PointsByPhase: want 2 entries, got %d", len(body.PointsByPhase))
	}
	if body.LastPredictionAt != nil {
		t.Error("LastPredictionAt: want nil when service returns nil")
	}
}

func TestUserStatsHandler_GetMyStats_200_LastPredictionAt_OmittedWhenNil(t *testing.T) {
	svc := &stubUserStatsSvc{stats: &domain.UserStats{PointsByPhase: map[domain.MatchPhase]int{}}}
	h := handler.NewUserStatsHandler(svc, zaptest.NewLogger(t))
	router := testUserStatsRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, pathUserStats, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if _, ok := raw["last_prediction_at"]; ok {
		t.Error("last_prediction_at should be omitted from JSON when nil")
	}
}

func TestUserStatsHandler_GetMyStats_200_LastPredictionAt_PresentWhenSet(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	svc := &stubUserStatsSvc{stats: &domain.UserStats{
		TotalPredictions: 3,
		PointsByPhase:    map[domain.MatchPhase]int{},
		LastPredictionAt: &ts,
	}}
	h := handler.NewUserStatsHandler(svc, zaptest.NewLogger(t))
	router := testUserStatsRouter(h, &domain.User{ID: 1})

	req := httptest.NewRequest(http.MethodGet, pathUserStats, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf(fmtExpect200, rr.Code)
	}
	var body handler.UserStatsResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf(fmtDecodeFail, err)
	}
	if body.LastPredictionAt == nil {
		t.Fatal("LastPredictionAt: want non-nil, got nil")
	}
	if *body.LastPredictionAt != "2026-06-15T18:00:00Z" {
		t.Errorf("LastPredictionAt: got %q; want \"2026-06-15T18:00:00Z\"", *body.LastPredictionAt)
	}
}
