package service

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// TestCalculatePoints verifies the full scoring decision table:
//
//	5 pts — exact scoreline
//	3 pts — correct non-draw outcome + correct goal margin
//	2 pts — correct non-draw outcome, wrong goal margin
//	2 pts — correct draw (no goal-difference bonus for draws)
//	0 pts — incorrect outcome
func TestCalculatePoints(t *testing.T) {
	const (
		fmtPoints = "calculatePoints(%d-%d | actual %d-%d): expected %d pts, got %d"
	)

	cases := []struct {
		name      string
		predHome  int
		predAway  int
		realHome  int
		realAway  int
		wantPts   int
	}{
		// ── Tier 1: exact scoreline ───────────────────────────────────────────
		{
			name: "exact_home_win",
			predHome: 2, predAway: 1,
			realHome: 2, realAway: 1,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name: "exact_away_win",
			predHome: 0, predAway: 3,
			realHome: 0, realAway: 3,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name: "exact_draw",
			predHome: 1, predAway: 1,
			realHome: 1, realAway: 1,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name: "exact_goalless_draw",
			predHome: 0, predAway: 0,
			realHome: 0, realAway: 0,
			wantPts: domain.PointsExactScore, // 5
		},

		// ── Tier 2a: correct non-draw outcome + correct goal margin ───────────
		{
			name: "home_win_correct_margin_1",
			predHome: 1, predAway: 0,
			realHome: 2, realAway: 1, // both margin = 1
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name: "home_win_correct_margin_2",
			predHome: 2, predAway: 0,
			realHome: 3, realAway: 1, // both margin = 2
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name: "away_win_correct_margin_1",
			predHome: 0, predAway: 1,
			realHome: 1, realAway: 2, // both margin = 1
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name: "away_win_correct_margin_3",
			predHome: 0, predAway: 3,
			realHome: 1, realAway: 4, // both margin = 3
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},

		// ── Tier 2b: correct non-draw outcome, wrong goal margin ──────────────
		{
			name: "home_win_wrong_margin",
			predHome: 3, predAway: 0, // margin 3
			realHome: 2, realAway: 1, // margin 1
			wantPts: domain.PointsCorrectOutcome, // 2
		},
		{
			name: "away_win_wrong_margin",
			predHome: 0, predAway: 1, // margin 1
			realHome: 0, realAway: 3, // margin 3
			wantPts: domain.PointsCorrectOutcome, // 2
		},

		// ── Tier 2c: correct draw — no goal-difference bonus ──────────────────
		{
			name: "correct_draw_no_margin_bonus",
			predHome: 2, predAway: 2,
			realHome: 1, realAway: 1, // both draw, both margin 0 — bonus NOT awarded
			wantPts: domain.PointsCorrectOutcome, // 2, not 3
		},
		{
			name: "correct_draw_different_scores",
			predHome: 3, predAway: 3,
			realHome: 0, realAway: 0,
			wantPts: domain.PointsCorrectOutcome, // 2
		},

		// ── Tier 3: incorrect outcome ─────────────────────────────────────────
		{
			name: "predicted_home_win_actual_draw",
			predHome: 2, predAway: 0,
			realHome: 1, realAway: 1,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name: "predicted_draw_actual_home_win",
			predHome: 1, predAway: 1,
			realHome: 2, realAway: 0,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name: "predicted_home_win_actual_away_win",
			predHome: 2, predAway: 1,
			realHome: 0, realAway: 1,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name: "predicted_away_win_actual_draw",
			predHome: 0, predAway: 2,
			realHome: 2, realAway: 2,
			wantPts: domain.PointsIncorrectResult, // 0
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pred := &domain.Prediction{HomeScore: tc.predHome, AwayScore: tc.predAway}
			got := calculatePoints(pred, tc.realHome, tc.realAway)
			if got != tc.wantPts {
				t.Errorf(fmtPoints,
					tc.predHome, tc.predAway,
					tc.realHome, tc.realAway,
					tc.wantPts, got,
				)
			}
		})
	}
}

// TestGoalDiff verifies the absolute-value goal-margin helper.
func TestGoalDiff(t *testing.T) {
	cases := []struct{ home, away, want int }{
		{3, 1, 2},
		{1, 3, 2},
		{0, 0, 0},
		{1, 1, 0},
		{5, 0, 5},
	}
	for _, tc := range cases {
		if got := goalDiff(tc.home, tc.away); got != tc.want {
			t.Errorf("goalDiff(%d, %d) = %d, want %d", tc.home, tc.away, got, tc.want)
		}
	}
}
