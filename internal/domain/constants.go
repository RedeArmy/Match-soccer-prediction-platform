package domain

import "time"

// Scoring rules define the points awarded per prediction outcome.
//
// The full matrix, from best to worst:
//
//   PointsExactScore (5)
//     — player predicts the exact scoreline (e.g. 2–1 → 2–1).
//
//   PointsCorrectOutcome (2) [+ PointsGoalDifference (1) when applicable]
//     — player predicts the correct winner or draw but not the exact score.
//     — an extra PointsGoalDifference point is added when the predicted
//       goal margin equals the actual margin (e.g. predict 2–0, result 3–1:
//       both are a 2-goal home win). This bonus does NOT apply to draws
//       because every draw has a goal difference of 0, making the check
//       trivially true and not a meaningful prediction skill.
//
//   PointsIncorrectResult (0)
//     — player predicts the wrong outcome, or no prediction was submitted.
//
// These constants are the single source of truth referenced by MatchScorer;
// no other package should hard-code scoring values.
const (
	PointsExactScore      = 5
	PointsCorrectOutcome  = 2
	PointsGoalDifference  = 1 // bonus for correct goal margin on non-draw results
	PointsIncorrectResult = 0
)

// PredictionDeadlineOffset is the duration before kick-off after which
// predictions are no longer accepted.
//
// The deadline is KickoffAt − PredictionDeadlineOffset. Any submission or
// update that arrives after that moment is rejected by domain.ValidatePrediction.
// Five minutes gives the system time to process last-second changes while
// preventing players from adjusting predictions once team news (e.g. a
// goalkeeper injury) becomes public in the stadium.
const PredictionDeadlineOffset = 5 * time.Minute

// MaxPredictionsPerUser is the maximum number of match predictions a single
// user may hold at once. A value of 0 means unlimited.
const MaxPredictionsPerUser = 0
