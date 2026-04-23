package domain

import (
	"regexp"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// emailRE is a structural check that catches obvious mistakes (missing "@",
// missing domain, empty local part). Full RFC 5322 compliance is intentionally
// not attempted here — Clerk already validates email format at signup, so this
// check is a defence-in-depth layer, not the primary gate.
var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// ValidateEmail returns a validation error when email is empty or fails the
// basic structural check. Call this wherever user email data enters the system
// (webhook handler, user creation endpoints).
func ValidateEmail(email string) error {
	if email == "" {
		return apperrors.Validation("email must not be empty")
	}
	if !emailRE.MatchString(email) {
		return apperrors.Validation("email is not a valid address")
	}
	return nil
}

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
// deadlineOffset is subtracted from kickoffAt to derive the closing time;
// pass PredictionDeadlineOffset as the default or read a runtime value from
// SystemParamService. Accepting now and deadlineOffset as parameters makes the
// function fully deterministic: tests can inject any reference time and offset
// without racing against the real clock.
func ValidatePrediction(p *Prediction, kickoffAt, now time.Time, deadlineOffset time.Duration) error {
	if p.HomeScore < 0 {
		return apperrors.Validation("predicted home score must not be negative")
	}
	if p.AwayScore < 0 {
		return apperrors.Validation("predicted away score must not be negative")
	}
	deadline := kickoffAt.Add(-deadlineOffset)
	if now.After(deadline) {
		return apperrors.Validation("predictions are no longer accepted for this match")
	}
	return nil
}

// ValidateQuiniela checks that the essential fields of a Quiniela are present
// and that PrizeThreshold is positive when provided.
func ValidateQuiniela(q *Quiniela) error {
	if q.Name == "" {
		return apperrors.Validation("quiniela name must not be empty")
	}
	if q.OwnerID == 0 {
		return apperrors.Validation("quiniela must have an owner")
	}
	if q.PrizeThreshold < 1 {
		return apperrors.Validation("prize_threshold must be at least 1")
	}
	return nil
}

// validPhases is the set of recognised MatchPhase values. It is used by
// ValidateMatchPhase to reject arbitrary strings before they reach the DB.
var validPhases = map[MatchPhase]struct{}{
	PhaseGroupStage:   {},
	PhaseRoundOf32:    {},
	PhaseRoundOf16:    {},
	PhaseQuarterFinal: {},
	PhaseSemiFinal:    {},
	PhaseThirdPlace:   {},
	PhaseFinal:        {},
}

// ValidateMatchPhase returns a validation error when phase is not one of the
// recognised FIFA World Cup 2026 tournament phases. An empty string is treated
// as "no phase filter" and is therefore valid (returns nil).
func ValidateMatchPhase(phase MatchPhase) error {
	if phase == "" {
		return nil
	}
	if _, ok := validPhases[phase]; !ok {
		return apperrors.Validation("phase is not a recognised tournament phase")
	}
	return nil
}
