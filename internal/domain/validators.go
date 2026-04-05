package domain

import (
	"time"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ValidateMatch checks that the essential fields of a Match are coherent
// before the entity is persisted for the first time.
//
// This validates business invariants, not HTTP request structure. The handler
// layer is responsible for confirming that the JSON body is well-formed; this
// function confirms that the decoded values make sense for the domain.
func ValidateMatch(m *Match) error {
	if m.HomeTeam == "" {
		return apperrors.Validation("home team must not be empty")
	}
	if m.AwayTeam == "" {
		return apperrors.Validation("away team must not be empty")
	}
	if m.HomeTeam == m.AwayTeam {
		return apperrors.Validation("home team and away team must be different")
	}
	if m.KickoffAt.IsZero() {
		return apperrors.Validation("kick-off time must be set")
	}
	return nil
}

// ValidateMatchResult checks that the supplied score pointers form a valid
// final result before they are persisted on an existing Match.
func ValidateMatchResult(homeScore, awayScore *int) error {
	if homeScore == nil || awayScore == nil {
		return apperrors.Validation("home score and away score must both be provided")
	}
	if *homeScore < 0 {
		return apperrors.Validation("home score must not be negative")
	}
	if *awayScore < 0 {
		return apperrors.Validation("away score must not be negative")
	}
	return nil
}

// ValidatePrediction checks that a Prediction carries a plausible scoreline
// and that it was submitted before the match deadline.
//
// The caller must pass the KickoffAt of the corresponding Match so that the
// deadline check can be performed here rather than scattered across services.
func ValidatePrediction(p *Prediction, kickoffAt time.Time) error {
	if p.HomeScore < 0 {
		return apperrors.Validation("predicted home score must not be negative")
	}
	if p.AwayScore < 0 {
		return apperrors.Validation("predicted away score must not be negative")
	}
	deadline := kickoffAt.Add(-PredictionDeadlineOffset)
	if time.Now().After(deadline) {
		return apperrors.Validation("predictions are no longer accepted for this match")
	}
	return nil
}

// ValidateQuiniela checks that the essential fields of a Quiniela are present.
func ValidateQuiniela(q *Quiniela) error {
	if q.Name == "" {
		return apperrors.Validation("quiniela name must not be empty")
	}
	if q.OwnerID == 0 {
		return apperrors.Validation("quiniela must have an owner")
	}
	return nil
}
