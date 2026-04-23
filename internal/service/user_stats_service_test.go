package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

const (
	statsUnexpectedErrFmt = "unexpected error: %v"
	statsCurLngFmt        = "expected current/longest, got %d/%d"
	statsLongestFmt       = "expected longest streak, got %d"
	statsExpectErrMsg     = "expected error, got nil"
)

// ── stubUserStatsPredRepo ─────────────────────────────────────────────────────

// stubUserStatsPredRepo embeds stubPredRepo and overrides the three methods
// used exclusively by UserStatsService.
type stubUserStatsPredRepo struct {
	stubPredRepo
	counts       *domain.UserPredictionCounts
	countsErr    error
	byPhase      map[domain.MatchPhase]int
	byPhaseErr   error
	chronoPoints []int
	chronoErr    error
}

func (r *stubUserStatsPredRepo) GetUserPredictionCounts(_ context.Context, _ int) (*domain.UserPredictionCounts, error) {
	return r.counts, r.countsErr
}
func (r *stubUserStatsPredRepo) GetUserPointsByPhase(_ context.Context, _ int) (map[domain.MatchPhase]int, error) {
	return r.byPhase, r.byPhaseErr
}
func (r *stubUserStatsPredRepo) ListUserScoredPointsChronological(_ context.Context, _ int) ([]int, error) {
	return r.chronoPoints, r.chronoErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func defaultCounts() *domain.UserPredictionCounts {
	return &domain.UserPredictionCounts{
		TotalPredictions:   10,
		ScoredPredictions:  8,
		CorrectPredictions: 5,
		ExactPredictions:   2,
		TotalPoints:        16,
	}
}

// ── GetMyStats ────────────────────────────────────────────────────────────────

func TestGetMyStats_HappyPath_ReturnsPopulatedStats(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := &stubUserStatsPredRepo{
		counts: &domain.UserPredictionCounts{
			TotalPredictions:   10,
			ScoredPredictions:  8,
			CorrectPredictions: 5,
			ExactPredictions:   2,
			TotalPoints:        16,
			LastPredictionAt:   &now,
		},
		byPhase: map[domain.MatchPhase]int{
			domain.PhaseGroupStage:   10,
			domain.PhaseQuarterFinal: 6,
		},
		chronoPoints: []int{2, 0, 2, 5, 2},
	}
	svc := NewUserStatsService(repo)

	stats, err := svc.GetMyStats(context.Background(), 1)
	if err != nil {
		t.Fatalf(statsUnexpectedErrFmt, err)
	}

	if stats.TotalPredictions != 10 {
		t.Errorf("TotalPredictions: want 10, got %d", stats.TotalPredictions)
	}
	if stats.ScoredPredictions != 8 {
		t.Errorf("ScoredPredictions: want 8, got %d", stats.ScoredPredictions)
	}
	if stats.CorrectPredictions != 5 {
		t.Errorf("CorrectPredictions: want 5, got %d", stats.CorrectPredictions)
	}
	if stats.ExactPredictions != 2 {
		t.Errorf("ExactPredictions: want 2, got %d", stats.ExactPredictions)
	}
	if stats.TotalPoints != 16 {
		t.Errorf("TotalPoints: want 16, got %d", stats.TotalPoints)
	}
	// AccuracyPct = 5/8 * 100 = 62.5
	if stats.AccuracyPct != 62.5 {
		t.Errorf("AccuracyPct: want 62.5, got %f", stats.AccuracyPct)
	}
	// AvgPointsPerPred = 16/8 = 2.0
	if stats.AvgPointsPerPred != 2.0 {
		t.Errorf("AvgPointsPerPred: want 2.0, got %f", stats.AvgPointsPerPred)
	}
	if len(stats.PointsByPhase) != 2 {
		t.Errorf("PointsByPhase: want 2 entries, got %d", len(stats.PointsByPhase))
	}
	// chronoPoints [2,0,2,5,2]: current = trailing run 2,5,2 = 3; longest = 3
	if stats.CurrentStreak != 3 {
		t.Errorf("CurrentStreak: want 3, got %d", stats.CurrentStreak)
	}
	if stats.LongestStreak != 3 {
		t.Errorf("LongestStreak: want 3, got %d", stats.LongestStreak)
	}
	if stats.LastPredictionAt == nil {
		t.Error("LastPredictionAt: want non-nil, got nil")
	}
}

func TestGetMyStats_NoScoredPredictions_RatesAreZero(t *testing.T) {
	repo := &stubUserStatsPredRepo{
		counts:       &domain.UserPredictionCounts{TotalPredictions: 3},
		byPhase:      map[domain.MatchPhase]int{},
		chronoPoints: nil,
	}
	svc := NewUserStatsService(repo)

	stats, err := svc.GetMyStats(context.Background(), 1)
	if err != nil {
		t.Fatalf(statsUnexpectedErrFmt, err)
	}
	if stats.AccuracyPct != 0 {
		t.Errorf("AccuracyPct: want 0, got %f", stats.AccuracyPct)
	}
	if stats.AvgPointsPerPred != 0 {
		t.Errorf("AvgPointsPerPred: want 0, got %f", stats.AvgPointsPerPred)
	}
	if stats.CurrentStreak != 0 || stats.LongestStreak != 0 {
		t.Errorf("streaks should both be 0 for empty history")
	}
	if stats.LastPredictionAt != nil {
		t.Error("LastPredictionAt: want nil, got non-nil")
	}
}

func TestGetMyStats_CountsRepoError_PropagatesError(t *testing.T) {
	repoErr := errors.New("db failure")
	repo := &stubUserStatsPredRepo{countsErr: repoErr}
	svc := NewUserStatsService(repo)

	_, err := svc.GetMyStats(context.Background(), 1)
	if err == nil {
		t.Fatal(statsExpectErrMsg)
	}
}

func TestGetMyStats_ByPhaseRepoError_PropagatesError(t *testing.T) {
	repoErr := errors.New("phase query failed")
	repo := &stubUserStatsPredRepo{
		counts:     defaultCounts(),
		byPhaseErr: repoErr,
	}
	svc := NewUserStatsService(repo)

	_, err := svc.GetMyStats(context.Background(), 1)
	if err == nil {
		t.Fatal(statsExpectErrMsg)
	}
}

func TestGetMyStats_ChronoRepoError_PropagatesError(t *testing.T) {
	repoErr := errors.New("chrono query failed")
	repo := &stubUserStatsPredRepo{
		counts:    defaultCounts(),
		byPhase:   map[domain.MatchPhase]int{},
		chronoErr: repoErr,
	}
	svc := NewUserStatsService(repo)

	_, err := svc.GetMyStats(context.Background(), 1)
	if err == nil {
		t.Fatal(statsExpectErrMsg)
	}
}

func TestGetMyStats_AccuracyRounding(t *testing.T) {
	// 1 correct / 3 scored = 33.33...%  rounds to 33.33
	repo := &stubUserStatsPredRepo{
		counts: &domain.UserPredictionCounts{
			ScoredPredictions:  3,
			CorrectPredictions: 1,
			TotalPoints:        2,
		},
		byPhase:      map[domain.MatchPhase]int{},
		chronoPoints: []int{2, 0, 0},
	}
	svc := NewUserStatsService(repo)

	stats, err := svc.GetMyStats(context.Background(), 1)
	if err != nil {
		t.Fatalf(statsUnexpectedErrFmt, err)
	}
	if stats.AccuracyPct != 33.33 {
		t.Errorf("AccuracyPct: want 33.33, got %f", stats.AccuracyPct)
	}
}

// ── computeStreaks ─────────────────────────────────────────────────────────────

func TestComputeStreaks_EmptySlice_BothZero(t *testing.T) {
	cur, lng := computeStreaks(nil)
	if cur != 0 || lng != 0 {
		t.Errorf(statsCurLngFmt, cur, lng)
	}
}

func TestComputeStreaks_AllCorrect(t *testing.T) {
	cur, lng := computeStreaks([]int{2, 5, 2, 2, 5})
	if cur != 5 {
		t.Errorf("current: want 5, got %d", cur)
	}
	if lng != 5 {
		t.Errorf(statsLongestFmt, lng)
	}
}

func TestComputeStreaks_AllZero_BothZero(t *testing.T) {
	cur, lng := computeStreaks([]int{0, 0, 0})
	if cur != 0 || lng != 0 {
		t.Errorf(statsCurLngFmt, cur, lng)
	}
}

func TestComputeStreaks_CurrentBrokenByFinalZero(t *testing.T) {
	// 5 correct then a zero: current=0, longest=5
	cur, lng := computeStreaks([]int{2, 5, 2, 2, 5, 0})
	if cur != 0 {
		t.Errorf("current: want 0, got %d", cur)
	}
	if lng != 5 {
		t.Errorf(statsLongestFmt, lng)
	}
}

func TestComputeStreaks_CurrentLongerThanHistoricalBest(t *testing.T) {
	// 2 correct, 0, then 4 correct: current=4, longest=4
	cur, lng := computeStreaks([]int{5, 2, 0, 2, 2, 5, 2})
	if cur != 4 {
		t.Errorf("current: want 4, got %d", cur)
	}
	if lng != 4 {
		t.Errorf("longest: want 4, got %d", lng)
	}
}

func TestComputeStreaks_LongestEarlierThanCurrent(t *testing.T) {
	// 5 correct, 0, 2 correct: longest=5, current=2
	cur, lng := computeStreaks([]int{2, 2, 5, 2, 2, 0, 5, 2})
	if cur != 2 {
		t.Errorf("current: want 2, got %d", cur)
	}
	if lng != 5 {
		t.Errorf(statsLongestFmt, lng)
	}
}

func TestComputeStreaks_SingleCorrect(t *testing.T) {
	cur, lng := computeStreaks([]int{5})
	if cur != 1 || lng != 1 {
		t.Errorf("want 1,1; got %d,%d", cur, lng)
	}
}

func TestComputeStreaks_SingleZero(t *testing.T) {
	cur, lng := computeStreaks([]int{0})
	if cur != 0 || lng != 0 {
		t.Errorf(statsCurLngFmt, cur, lng)
	}
}

// ── round2dp ──────────────────────────────────────────────────────────────────

func TestRound2dp(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{33.333333, 33.33},
		{62.5, 62.5},
		{100.0, 100.0},
		{0.0, 0.0},
		{2.005, 2.01},
		{66.666666, 66.67},
	}
	for _, tc := range cases {
		got := round2dp(tc.in)
		if got != tc.want {
			t.Errorf("round2dp(%f): want %f, got %f", tc.in, tc.want, got)
		}
	}
}
